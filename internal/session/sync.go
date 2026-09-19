// Package session keeps the repo's Claude Code skills pinned. It writes them
// from their upstreams into .claude/skills, checks that the copy on disk still
// matches the pins, and proves that a fresh session can actually load them.
//
// What is pinned lives in session.toml; this package only reads it.
package session

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joeblew999/dev/cli"
)

// Skills live in the repo so every Claude Code session here loads them, and are
// pinned so they never drift from the tools they describe.
const (
	skillsDir = ".claude/skills"
	lockFile  = "SKILLS.lock"
	pinsFile  = "session.toml"
)

// skillFiles is a skill set: path relative to the skills directory -> contents.
type skillFiles map[string][]byte

// Sync writes the pinned skills into .claude/skills and reports sessions that
// must restart to see them. Files it does not own are left alone, so skills
// this repo writes itself survive a sync.
func Sync(out io.Writer) error {
	want, err := pinnedSkills()
	if err != nil {
		return err
	}
	existed := dirExists(skillsDir)
	have, err := readDir(skillsDir)
	if err != nil {
		return err
	}
	if err := writeOwned(skillsDir, have, want); err != nil {
		return err
	}

	fmt.Fprintf(out, "skills in %s:\n", skillsDir)
	fmt.Fprint(out, cli.Indent(string(want[lockFile])))
	if !existed {
		fmt.Fprintf(out, "\nThese are new: Claude Code reads %s at startup.\n", skillsDir)
	}
	p, err := loadPins()
	if err != nil {
		return err
	}
	if err := syncSettings(out, p.Claude); err != nil {
		return err
	}
	if !sameFiles(have, want) {
		warnStaleSessions(out, time.Now())
	}
	return nil
}

// Check fails when the skills on disk differ from the lock, or when a go.mod
// disagrees with the mise pin that is the source of truth for its version. It
// needs no network: file contents are compared against the hashes sync recorded
// in the lock.
func Check(c cli.Call) error {
	if err := c.CheckReportFlags(); err != nil {
		return err
	}
	// The pins are read once, before anything is checked against them: a
	// file that cannot be read is not three findings, it is one reason
	// nothing could be checked.
	pins, err := loadPins()
	if err != nil {
		return err
	}
	started := time.Now()
	rep := cli.NewReport("session", skillsDir)
	for _, part := range []struct {
		name, provides string
		look           func() ([]cli.Finding, string, error)
	}{
		{"skills", "the vendored skills are the ones session.toml pins", lockedSkills},
		{"settings", ".claude/settings.json holds what [claude] implies",
			func() ([]cli.Finding, string, error) { return settingsFindings(pins.Claude) }},
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
	rep.Fail = skillsDir + " does not match session.toml; fix with: " + syncCmd
	return c.Finish(rep, started, func(r *cli.Report) { writeReport(c, r) })
}

// lockedSkills is the vendored skills against what the lock records.
func lockedSkills() ([]cli.Finding, string, error) {
	have, err := readDir(skillsDir)
	if err != nil {
		return nil, "", err
	}
	want, err := lockedFiles()
	if err != nil {
		return nil, "", err
	}
	return findings("skill-drift", diffLocked(have, want),
		skillsDir+" does not match its pins"), cli.Plural(len(want), "file"), nil
}

// findings turns a diff — the missing/changed/unexpected lines every check
// here produces — into what a report carries. Three checks had their own
// sentence around the same list.
func findings(id string, diff []string, what string) []cli.Finding {
	return cli.Map(diff, func(line string) cli.Finding {
		return cli.Finding{Severity: cli.SevError, ID: id, Message: line,
			Fix: what + "; fix with: " + syncCmd}
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

// pinnedSkills collects every skill at its pinned version. The lock records
// one line per skill (its source) plus one line per file (its hash), so check
// can verify the files without downloading anything.
func pinnedSkills() (skillFiles, error) {
	p, err := loadPins()
	if err != nil {
		return nil, err
	}
	files := skillFiles{}
	var lock []string

	for _, source := range p.names() {
		s := p.Source[source]
		archive, err := download(fmt.Sprintf("https://codeload.github.com/%s/tar.gz/%s", s.Repo, s.Ref))
		if err != nil {
			return nil, err
		}
		prefix := fmt.Sprintf("skills-%s/skills/", s.Ref)
		for _, name := range s.Skills {
			if err := copyTar(files, archive, prefix+name+"/", name, s.Repo, s.Ref); err != nil {
				return nil, err
			}
			lock = append(lock, lockSkill(name, fmt.Sprintf("github.com/%s@%s", s.Repo, s.Ref[:12]), files))
		}
	}

	files[lockFile] = []byte(strings.Join(cli.Sorted(lock), "\n") + "\n")
	return files, nil
}

// writeOwned writes want into dir, deleting only files sync owned before:
// anything in have that is neither in want nor owned by the previous sync is
// left alone. Owned paths come from the previous lock: every top-level skill
// directory it names, plus the lock file itself.
func writeOwned(dir string, have, want skillFiles) error {
	prefixes := ownedPaths(have[lockFile])
	for name := range have {
		if _, ok := want[name]; ok {
			continue
		}
		if !isOwned(name, prefixes) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			return err
		}
	}
	for name, data := range want {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			return err
		}
	}
	removeEmptyDirs(dir)
	return nil
}

// ownedPaths returns the top-level paths the previous sync owned: the lock
// file and every skill directory it names.
func ownedPaths(lock []byte) []string {
	owned := []string{lockFile}
	for _, line := range cli.Lines(string(lock)) {
		if name, _, ok := strings.Cut(line, "\t"); ok && name != "" {
			owned = append(owned, name)
		}
	}
	return owned
}

func isOwned(name string, owned []string) bool {
	for _, prefix := range owned {
		if name == prefix || strings.HasPrefix(name, prefix+"/") {
			return true
		}
	}
	return false
}

// removeEmptyDirs deletes directories left empty after owned files moved out,
// so a dropped skill leaves no empty folder behind.
func removeEmptyDirs(dir string) {
	var dirs []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != dir {
			dirs = append(dirs, p)
		}
		return nil
	})
	// Deepest first, so a directory is removed after what it holds.
	for _, d := range cli.SortedDesc(dirs) {
		_ = os.Remove(d)
	}
}

// diffFiles describes how have differs from want, most useful first.
func diffFiles(have, want skillFiles) []string {
	return cli.DiffMaps(have, want, bytes.Equal)
}

func sameFiles(a, b skillFiles) bool { return len(diffFiles(a, b)) == 0 }

func dirExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
