package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skills is `<cmd> skills`: what every agent in this repo can read, and where
// each one came from.
//
// A skill arrives one of two ways and they are easy to confuse. The repo's own
// commands write theirs, committed, real directories. A tool the repo pins
// ships its own, and mise links those in per developer, gitignored. Told
// apart by whether the entry is a symlink, which is the same thing mise's
// prune uses to decide what it may remove.
//
// It reads the directories rather than asking an agent what it loaded,
// because an agent is told what is available and cannot enumerate it. The
// directory is the fact.
func (c Command) skills(call Call) error {
	if HelpRequested(call.Args) {
		return ErrHelp
	}
	if len(call.Args) > 0 {
		return call.Usagef("takes no arguments")
	}
	root, err := root(".")
	if err != nil {
		return err
	}
	seen := map[string][]string{}
	for _, dir := range []string{ClaudeDir, AgentsDir} {
		found := readSkills(filepath.Join(root, dir))
		fmt.Fprintf(call.Stdout, "%s\n", dir)
		if len(found) == 0 {
			fmt.Fprintf(call.Stdout, "  (none)\n")
		}
		for _, s := range found {
			fmt.Fprintf(call.Stdout, "  %-22s %s\n", s.name, s.from)
			seen[s.name] = append(seen[s.name], dir)
		}
		fmt.Fprintln(call.Stdout)
	}
	// An agent reads one of these directories and not the other, so a skill in
	// one alone is a skill that agent cannot see. Worth saying, because mise
	// syncs a pinned tool's skills into .claude and nowhere else.
	var only []string
	for name, dirs := range seen {
		if len(dirs) == 1 {
			only = append(only, name+" is in "+dirs[0]+" only")
		}
	}
	sort.Strings(only)
	for _, line := range only {
		fmt.Fprintf(call.Stdout, "%s\n", line)
	}
	return nil
}

// skill is one entry of an agent's skills directory.
type skillEntry struct{ name, from string }

// readSkills lists a skills directory, saying where each entry came from.
func readSkills(dir string) []skillEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []skillEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, skillEntry{e.Name(), source(filepath.Join(dir, e.Name()))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// source is where a skill came from: a symlink is a tool this repo pins, and
// the version is in the path mise linked to. Anything else the repo wrote.
func source(path string) string {
	fi, err := os.Lstat(path)
	if err != nil {
		return "unreadable"
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return "this repo"
	}
	target, err := os.Readlink(path)
	if err != nil {
		return "a pinned tool"
	}
	// mise links into <install>/<tool>/<version>/skills/<name>.
	parts := strings.Split(filepath.ToSlash(target), "/")
	for i, p := range parts {
		if p == "skills" && i >= 2 {
			return "pinned: " + parts[i-2] + " " + parts[i-1]
		}
	}
	return "a pinned tool"
}

// mirror makes .agents/skills carry what .claude/skills carries.
//
// mise links a pinned tool's skill into one directory — skills.dir, which is
// .claude/skills and is a single path, so mise cannot serve both agents. Left
// alone, every skill a repo pins reaches Claude Code and none reaches Copilot,
// which is a silent half of the repo's own convention.
//
// So a link in one becomes a link in the other, pointing at the same install,
// and a mirror whose original is gone is removed — because mise's prune takes
// the original when a pin changes, and a mirror nobody prunes is a skill at a
// version nothing pins any more.
//
// Only symlinks are touched. A real directory in either place is the repo's
// own manual, written by skill, and is none of this function's business.
func mirror(root string, out io.Writer) error {
	from, to := filepath.Join(root, ClaudeDir), filepath.Join(root, AgentsDir)
	want := map[string]string{}
	for _, e := range readSkills(from) {
		target, err := os.Readlink(filepath.Join(from, e.name))
		if err != nil {
			continue // a real directory: the repo's own
		}
		want[e.name] = target
	}
	for _, e := range readSkills(to) {
		p := filepath.Join(to, e.name)
		if _, err := os.Readlink(p); err != nil {
			continue
		}
		if _, ok := want[e.name]; !ok {
			if err := os.Remove(p); err != nil {
				return err
			}
			fmt.Fprintf(out, "removed %s; nothing pins it any more\n", rel(p))
		}
	}
	if len(want) > 0 {
		if err := os.MkdirAll(to, 0o755); err != nil {
			return err
		}
	}
	var made []string
	for name, target := range want {
		p := filepath.Join(to, name)
		made = append(made, filepath.Join(AgentsDir, name))
		if have, err := os.Readlink(p); err == nil && have == target {
			continue
		}
		os.Remove(p)
		if err := os.Symlink(target, p); err != nil {
			return err
		}
		fmt.Fprintf(out, "linked %s, so both agents read it\n", rel(p))
	}
	// A mirror is written per developer from the versions that repo pins, the
	// same as the links it mirrors, so it is ignored for the same reason. The
	// code that writes it is the code that ignores it.
	sort.Strings(made)
	return Ignore(root, made...)
}
