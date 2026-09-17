// Putting a manual where it is read: the three copies of it, where the repo
// root is, and the skill and version verbs every command gets. Nothing here
// decides what a manual says; render.go does that.
package cli

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// paths are the manual's copies: the one the release ships, and the ones
// agents read in this repo — Claude Code's .claude/skills and Copilot's
// .agents/skills. All sit under the repo root, found by walking up to
// mise.toml — every repo on the stack has one at its root — and never
// through git: a Worker's main links this package into its wasm.
func (c Command) paths() (shipped, claude, agents string, err error) {
	root, err := root(".")
	if err != nil {
		return "", "", "", err
	}
	return filepath.Join(root, ShippedDir, c.Name, SkillFile),
		filepath.Join(root, ClaudeDir, c.Name, SkillFile),
		filepath.Join(root, AgentsDir, c.Name, SkillFile), nil
}

// root is the nearest directory at or above dir holding a mise.toml.
func root(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, "mise.toml")); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("no mise.toml at or above %s; run this inside a repo on the stack", dir)
		}
		abs = parent
	}
}

// rel is p as a message shows it: relative to where the command runs.
func rel(p string) string {
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, p); err == nil {
			return r
		}
	}
	return p
}

// skill is `<Name> skill [--check]`: write every copy of the manual, or with
// --check say which is stale and how to fix it.
func (c Command) skill(call Call) error {
	check := call.Given("check")
	if len(call.Args) > 0 {
		return call.Usagef("takes only --check")
	}
	// Both branches below answer from prose compiled into this binary, so
	// neither means anything if the binary is behind its sources: writing
	// would rewrite every copy from old bytes and report success, and
	// checking would compare that old render against equally old files and
	// report "up to date". Refuse instead — the cost is one rebuild, and the
	// alternative is a wrong answer nobody can see.
	dir, err := root(".")
	if err != nil {
		return err
	}
	if changed := staleBuild(dir); changed != "" {
		what := "write a manual"
		if check {
			what = "check a manual"
		}
		return fmt.Errorf("%s has changed since this %s was built, so it would %s from stale embedded prose; rebuild first: mise run build", rel(changed), c.Name, what)
	}
	// Both agent directories carry the same pinned skills, which mise cannot
	// do for itself: its skills.dir is one path.
	if !check {
		if err := mirror(dir, call.Stdout); err != nil {
			return err
		}
	}
	for name, body := range c.Skills {
		if err := c.writeSkill(call.Stdout, name, withProvenance(body, c.Name), check); err != nil {
			return err
		}
	}
	want := c.render()
	shipped, claude, agents, err := c.paths()
	if err != nil {
		return err
	}
	for _, p := range []string{shipped, claude, agents} {
		if check {
			have, err := os.ReadFile(p)
			if err != nil || string(have) != want {
				return fmt.Errorf("%s is stale; regenerate it with: %s skill", rel(p), c.Name)
			}
			fmt.Fprintf(call.Stdout, "%s is up to date\n", rel(p))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(want), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(call.Stdout, "wrote %s from the verbs' own usage\n", rel(p))
	}
	return nil
}

// writeSkill writes or checks one of c.Skills, in the three places a manual
// goes. It is the same work c.skill does for the command's own manual, on a
// body that was written rather than rendered.
func writeSkillTo(out io.Writer, paths []string, body string, check bool) error {
	for _, p := range paths {
		if check {
			have, err := os.ReadFile(p)
			if err != nil || string(have) != body {
				return fmt.Errorf("%s is stale; regenerate it with: dev skill", rel(p))
			}
			fmt.Fprintf(out, "%s is up to date\n", rel(p))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "wrote %s\n", rel(p))
	}
	return nil
}

// writeSkill resolves where a named skill goes, then writes or checks it.
func (c Command) writeSkill(stdout io.Writer, name, body string, check bool) error {
	root, err := root(".")
	if err != nil {
		return err
	}
	return writeSkillTo(stdout, skillPaths(root, name), body, check)
}

// skillPaths are the three copies of a manual for name: the one the release
// ships, and the ones each agent reads in this repo.
func skillPaths(root, name string) []string {
	return []string{
		filepath.Join(root, ShippedDir, name, SkillFile),
		filepath.Join(root, ClaudeDir, name, SkillFile),
		filepath.Join(root, AgentsDir, name, SkillFile),
	}
}

// version is `<Name> version`.
func (c Command) version(call Call) error {
	if len(call.Args) > 0 {
		return call.Usagef("takes no arguments")
	}
	fmt.Fprintln(call.Stdout, c.Version)
	return nil
}

// checkFlag is `skill --check`: say whether every copy is current, write none.
func checkFlag(fs *flag.FlagSet) {
	fs.Var(new(Bool), "check", "say whether each copy is current; write nothing")
}
