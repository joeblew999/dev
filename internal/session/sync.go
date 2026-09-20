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
		return err
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

	fmt.Fprintf(out, "skills in %s:\n", english(vendored.Dirs()))
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
	rep := cli.NewReport("session", english(vendored.Dirs()))
	for _, part := range []struct {
		name, provides string
		look           func() ([]cli.Finding, string, error)
	}{
		{"skills", "every agent's directory holds what session.toml pins", lockedSkills},
		{"settings", ".claude/settings.json holds what [claude] implies",
			func() ([]cli.Finding, string, error) { return settingsFindings(p.Claude) }},
		{"paths", "nothing committed names this machine's home directory", portableFindings},
	} {
		at := time.Now()
		found, covered, err := part.look()
		took := time.Since(at)
		step := cli.Step{Name: part.name, Provides: part.provides, Covered: covered,
			Took: cli.Took(took), TookMs: took.Milliseconds(), Findings: len(found)}
		if err != nil {
			rep.NotRun(step, err.Error())
			continue
		}
		for _, f := range found {
			f.Tool = part.name
			rep.Add(f)
		}
		rep.Ran(step)
	}
	warnStaleSessions(c.Stderr, time.Now())
	rep.Fail = "what agents read does not match " + pins.File + "; fix with: " + pins.SyncCommand()
	return c.Finish(rep, started, func(r *cli.Report) { writeReport(c, r) })
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

// writeReport is the human answer: what was checked, then what is wrong.
func writeReport(c cli.Call, r *cli.Report) {
	for _, s := range r.Steps {
		fmt.Fprintf(c.Stdout, "  %-10s %7s  %-34s %s\n", s.Name, s.Took,
			cli.Or(s.Covered, s.Note), cli.Plural(s.Findings, "problem"))
	}
	if len(r.Findings) > 0 {
		fmt.Fprintln(c.Stdout)
	}
	for _, f := range r.Findings {
		fmt.Fprintf(c.Stdout, "%-8s %s (%s)\n  %s\n  fix: %s\n\n", f.Severity, f.ID, f.Tool, f.Message, f.Fix)
	}
	fmt.Fprintf(c.Stdout, "%s in %s: %s\n", r.Outcome, r.Took,
		cli.Plural(r.BySeverity[cli.SevError], "problem"))
}

// english joins paths the way a sentence does, so a message names every
// destination rather than the first one and an etcetera.
func english(items []string) string {
	switch len(items) {
	case 0:
		return "nowhere"
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return items[0] + ", " + english(items[1:])
}
