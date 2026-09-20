// Package session keeps a repo's Claude Code session pinned: which skills an
// agent may load, which settings keys those skills need, and whether a real
// session agrees.
//
// The work is split so that each part can be wrong about one thing only:
//
//	pins      what the repo declared     — session.toml, a committed file
//	upstream  what a repo at a commit holds — the network
//	vendored  what is on disk for agents  — the filesystem
//
// This package composes them and owns nothing else. A question about where a
// skill came from is upstream's; a question about what is on disk is
// vendored's; and a question about what the repo asked for is pins'. When
// something here is confusing, it is because a fact was answered in the wrong
// one of those, which is a thing the compiler can now say.
package session

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/session/pins"
	"github.com/joeblew999/dev/internal/session/vendored"
)

// Sync writes the pinned skills into every directory an agent reads, and
// reports sessions that must restart to see them. Files it does not own are
// left alone, so skills this repo writes itself survive a sync.
func Sync(out io.Writer) error {
	p, err := pins.Load()
	if err != nil {
		return orphaned(err)
	}
	want, err := collect(p)
	if err != nil {
		return err
	}
	existed := vendored.Existing()
	have, err := vendored.Read(vendored.Primary())
	if err != nil {
		return err
	}
	if err := vendored.Write(want); err != nil {
		return err
	}

	fmt.Fprintf(out, "skills in %s:\n", cli.Or(cli.English(vendored.Dirs()), "nowhere"))
	fmt.Fprint(out, cli.Indent(string(want[vendored.LockFile])))
	if len(existed) < len(vendored.Dirs()) {
		fmt.Fprintf(out, "\nThese are new: an agent reads its own directory at startup.\n")
	}
	if err := syncSettings(out, p.Claude); err != nil {
		return err
	}
	if !vendored.Same(have, want) {
		warnStaleSessions(out, time.Now())
	}
	return nil
}

// Check fails when what is on disk differs from what the pins imply. It needs
// no network: file contents are compared against the hashes sync recorded.
func Check(c cli.Call) error {
	if err := c.CheckReportFlags(); err != nil {
		return err
	}
	// The pins are read once, before anything is checked against them: a file
	// that cannot be read is not three findings, it is one reason nothing
	// could be checked.
	p, err := pins.Load()
	if err != nil {
		return err
	}
	started := time.Now()
	rep := cli.NewReport("session", cli.Or(cli.English(vendored.Dirs()), "nowhere"))
	cli.Parts(rep, 1, []cli.Part{
		{Name: "skills", Provides: "every agent's directory holds what session.toml pins", Look: lockedSkills},
		{Name: "settings", Provides: ".claude/settings.json holds what [claude] implies",
			Look: func() ([]cli.Finding, string, error) { return settingsFindings(p.Claude) }},
		{Name: "paths", Provides: "nothing committed names this machine's home directory", Look: portableFindings},
	})
	warnStaleSessions(c.Stderr, time.Now())
	rep.Fail = "what agents read does not match " + pins.File + "; fix with: " + pins.SyncCommand()
	return c.Finish(rep, started, c.Write)
}

// lockedSkills is every agent's directory against what the lock records.
func lockedSkills() ([]cli.Finding, string, error) {
	want, err := vendored.Locked()
	if err != nil {
		return nil, "", err
	}
	diff, n, err := vendored.Diff(want)
	if err != nil {
		return nil, "", err
	}
	return findings("skill-drift", diff, "what an agent reads does not match its pins"),
		cli.Plural(n, "file") + " in each of " + cli.Plural(len(vendored.Dirs()), "directory"), nil
}

// findings turns a diff — the missing/changed/unexpected lines every check
// here produces — into what a report carries. Three checks had their own
// sentence around the same list.
func findings(id string, diff []string, what string) []cli.Finding {
	return cli.Map(diff, func(line string) cli.Finding {
		return cli.Finding{Severity: cli.SevError, ID: id, Message: line,
			Fix: what + "; fix with: " + pins.SyncCommand()}
	})
}

// orphaned answers the one case where "fix session.toml" is the wrong advice:
// the file is gone, and skills a previous sync wrote are still on disk. That
// is what deleting it looks like, and telling someone to fix a file they just
// deleted sends them in a circle while every vendored skill stays loaded.
//
// pins cannot say this — it knows nothing of what is on disk, which is the
// point of it. Here, where both are in reach, it can.
func orphaned(err error) error {
	if _, statErr := os.Stat(pins.File); !os.IsNotExist(statErr) {
		return err
	}
	went, lockErr := vendored.LockedNames()
	if lockErr != nil || len(went) == 0 {
		return err
	}
	return fmt.Errorf("there is no %s, and %s from an earlier sync are still in %s — nothing pins them now and nothing will remove them; take them back with: %s",
		pins.File, cli.Plural(len(went), "skill"), cli.English(vendored.Dirs()),
		pins.SwapVerb(pins.SyncCommand(), "sync", "remove"))
}
