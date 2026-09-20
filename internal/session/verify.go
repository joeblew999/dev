package session

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/session/pins"
	"github.com/joeblew999/dev/internal/session/vendored"

	"github.com/joeblew999/dev/cli/tool"
)

// Verify asks a fresh headless Claude Code session which skills it can see. The
// file check cannot catch a SKILL.md that is present but never loads (bad
// frontmatter, wrong folder name); this can. It starts its own session, so it
// does not depend on the session the developer is in.
// Verify asks a fresh headless Claude Code session which skills it can see and
// holds the answer against SESSION.lock, which records every skill the session
// is allowed to have. Files cannot answer this: a SKILL.md can be present and
// never load, and a skill can load that no file here mentions — a marketplace
// plugin, or one synced from claude.ai, which no project setting can block.
//
// update rewrites the lock from what the session reports, for when a Claude
// Code upgrade adds a built-in.
func Verify(out io.Writer, update bool) error {
	if _, err := exec.LookPath(ClaudeBin); err != nil {
		return errors.New("claude CLI not found; install Claude Code to run this check")
	}
	seen, answer, err := sessionSkills()
	if err != nil {
		return err
	}

	current := claudeVersion()
	if update {
		return record(out, seen, current, "%s now allows the %d skills this session reports (Claude Code %s):\n", sessionLockPath(), len(seen), current)
	}

	// Vendored skills that never load are the older failure, and worth their
	// own message: the fix is frontmatter, not the lock.
	locked, err := vendored.LockedNames()
	if err != nil {
		return err
	}
	present := cli.ToSet(seen)
	missingSkills := cli.Filter(locked, func(name string) bool { return !present[name] })
	if len(missingSkills) > 0 {
		return fmt.Errorf("a fresh Claude Code session cannot see: %s\nit answered:\n%scheck the SKILL.md frontmatter, then: "+pins.SyncCommand(),
			strings.Join(missingSkills, ", "), cli.Indent(answer))
	}

	allowed, recordedBy, err := sessionLock()
	if errors.Is(err, errNoLock) {
		// The first verify in a repo, at its first push: nothing is allowed
		// yet, so what the session has now is what it allows. Recorded, not
		// refused, so day one has no manual step.
		return record(out, seen, current, "%s did not exist; it now allows the %d skills this first session reports (Claude Code %s). Commit it.\n", sessionLockPath(), len(seen), current)
	}
	if err != nil {
		return err
	}
	gone, arrived := diffSession(allowed, seen)
	if len(gone)+len(arrived) > 0 && current != "" && current != recordedBy {
		// Claude Code itself changed since the lock was recorded, and its
		// built-in skills change with it. That is an upgrade, not a skill
		// sneaking in from a plugin or claude.ai, so the lock follows it and
		// says what moved rather than turning a push red.
		if err := writeSessionLock(seen, current); err != nil {
			return err
		}
		was := cmp.Or(recordedBy, "an unrecorded version")
		fmt.Fprintf(out, "Claude Code is %s; %s was recorded by %s, so it now follows this session.\n", current, sessionLockPath(), was)
		if len(arrived) > 0 {
			fmt.Fprintf(out, "arrived:\n%s", listed(arrived))
		}
		if len(gone) > 0 {
			fmt.Fprintf(out, "gone:\n%s", listed(gone))
		}
		fmt.Fprintf(out, "commit %s\n", sessionLockPath())
		return nil
	}
	if len(gone) > 0 {
		return fmt.Errorf("skills the lock allows are no longer in the session:\n%sif that is intended: %s --update",
			listed(gone), pins.VerifyCommand())
	}
	if len(arrived) > 0 {
		return fmt.Errorf("skills reached this session that %s does not allow:\n%s%s",
			sessionLockPath(), listed(arrived), arrivalAdvice(arrived))
	}

	if recordedBy == "" && current != "" {
		// An older lock without a version line: record it, so the next
		// upgrade is recognised as one.
		if err := writeSessionLock(seen, current); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s now records Claude Code %s; commit it.\n", sessionLockPath(), current)
	}
	fmt.Fprintf(out, "a fresh Claude Code session has exactly the %d skills %s allows:\n", len(seen), sessionLockPath())
	fmt.Fprint(out, listed(seen))
	return nil
}

// record writes the lock and says what it now allows. Three of verify's
// answers end exactly this way, differing only in the sentence above the
// list, and each had written the write, the check and the two prints again.
func record(out io.Writer, seen []string, current, format string, a ...any) error {
	if err := writeSessionLock(seen, current); err != nil {
		return err
	}
	fmt.Fprintf(out, format, a...)
	fmt.Fprint(out, listed(seen))
	return nil
}

// listed is a set of names as this package shows one: indented, one per line.
func listed(names []string) string { return cli.Indent(strings.Join(names, "\n")) }

// errNoLock is the first verify in a repo: verify records the lock then.
var errNoLock = errors.New("no session lock yet")

// claudeVersion is Claude Code's own version, "" when it cannot be read. A
// change in it is the one legitimate way the built-in skills change.
func claudeVersion() string {
	res, err := tool.Cmd{Bin: ClaudeBin, Quiet: true, Args: []string{"--version"}}.Capture()
	fields := strings.Fields(res.Out)
	if err != nil || len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// sessionSkills asks a fresh session what it can see, returning the names it
// reported and its raw answer.
func sessionSkills() ([]string, string, error) {
	res, err := tool.Cmd{
		Bin:     ClaudeBin,
		Args:    []string{"-p", "List the names of every skill available to you, one per line, nothing else."},
		Timeout: 2 * time.Minute,
	}.Capture()
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, "", errors.New("claude did not answer within 2 minutes")
	}
	if err != nil {
		return nil, "", fmt.Errorf("claude -p: %w", err)
	}
	answer := res.Out
	var names []string
	for _, line := range cli.Lines(answer) {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return cli.Sorted(names), answer, nil
}

// diffSession reports what the lock allows but the session lost, and what the
// session gained that the lock does not allow.
func diffSession(allowed, seen []string) (gone, arrived []string) {
	return cli.DiffSets(allowed, seen)
}

// arrivalAdvice names the fix for what showed up. A namespaced name came from a
// marketplace plugin and can be blocked; anything else is either a Claude Code
// built-in after an upgrade, or a skill synced from claude.ai, which no project
// setting can block — only noticing it is possible.
func arrivalAdvice(arrived []string) string {
	plugins := map[string]bool{}
	for _, name := range arrived {
		if plugin, _, ok := strings.Cut(name, ":"); ok && plugin != "" {
			plugins[plugin] = true
		}
	}
	if len(plugins) > 0 {
		names := cli.SortedKeys(plugins)
		return fmt.Sprintf("these came from the %s plugin(s); add them to blocked_plugins in %s, then: %s",
			strings.Join(names, ", "), pins.File, pins.SyncCommand())
	}
	return fmt.Sprintf("Claude Code has not changed, so these came from claude.ai (which no project setting can\nblock) or a plugin; if they are meant to be here: %s --update", pins.VerifyCommand())
}

// sessionLockFile records every skill a session is allowed to have: the ones
// this repo provides, plus whatever Claude Code ships. It lives beside the
// skills so one directory holds everything the session is pinned to.
const sessionLockFile = "SESSION.lock"

func sessionLockPath() string { return filepath.Join(vendored.Primary(), sessionLockFile) }

// versionLine marks which Claude Code recorded the lock.
const versionLine = "# claude "

// sessionLock reads the allowed set and the Claude Code version that recorded
// it ("" for a lock written before versions were recorded). Its absence is an
// error naming the fix, because an empty set would silently allow everything.
func sessionLock() (names []string, recordedBy string, err error) {
	data, err := os.ReadFile(sessionLockPath())
	if os.IsNotExist(err) {
		return nil, "", fmt.Errorf("%w: %s", errNoLock, sessionLockPath())
	}
	if err != nil {
		return nil, "", err
	}
	names, recordedBy = parseSessionLock(string(data))
	return names, recordedBy, nil
}

func parseSessionLock(data string) (names []string, recordedBy string) {
	for _, line := range cli.Lines(data) {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, versionLine):
			recordedBy = strings.TrimSpace(strings.TrimPrefix(line, versionLine))
		case line != "" && !strings.HasPrefix(line, "#"):
			names = append(names, line)
		}
	}
	return cli.Sorted(names), recordedBy
}

func writeSessionLock(names []string, recordedBy string) error {
	return os.WriteFile(sessionLockPath(), []byte(formatSessionLock(names, recordedBy)), 0o644)
}

func formatSessionLock(names []string, recordedBy string) string {
	return "# Every skill a session here is allowed to have, recorded by\n" +
		"# `" + pins.VerifyCommand() + " --update`. Verify fails on anything else that arrives\n" +
		"# while Claude Code stays at the version below: a plugin, or a skill synced\n" +
		"# from claude.ai, which no setting can block. A Claude Code upgrade changes\n" +
		"# the built-ins, and verify re-records the lock for it.\n" +
		versionLine + recordedBy + "\n" +
		strings.Join(names, "\n") + "\n"
}
