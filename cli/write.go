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
	"strings"
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
	if err := c.readme(call.Stdout, dir, check); err != nil {
		return err
	}
	want := c.render()
	shipped, claude, agents, err := c.paths()
	if err != nil {
		return err
	}
	for _, p := range []string{shipped, claude, agents} {
		written, err := put(call.Stdout, c.Name, p, want, check)
		if err != nil {
			return err
		}
		if !written {
			continue
		}
		fmt.Fprintf(call.Stdout, "wrote %s from the verbs' own usage\n", rel(p))
	}
	return nil
}

// writeSkill writes or checks one of c.Skills, in the three places a manual
// goes. It is the same work c.skill does for the command's own manual, on a
// body that was written rather than rendered.
func writeSkillTo(out io.Writer, name string, paths []string, body string, check bool) error {
	for _, p := range paths {
		written, err := put(out, name, p, body, check)
		if err != nil {
			return err
		}
		if written {
			fmt.Fprintf(out, "wrote %s\n", rel(p))
		}
	}
	return nil
}

// put writes one file, or with check says whether it is current. Every
// generated file this command owns goes through here: the three copies of a
// manual, a skill it ships, and the README's install block all had the same
// read-compare-or-write, and the wordings had already drifted — one named
// `dev skill` where another named the command.
//
// It reports whether anything was written, so a caller can say what it did
// in its own words without repeating the decision.
func put(out io.Writer, name, path, want string, check bool) (written bool, err error) {
	if check {
		have, err := os.ReadFile(path)
		if err != nil || string(have) != want {
			return false, fmt.Errorf("%s is stale; regenerate it with: %s skill", rel(path), name)
		}
		fmt.Fprintf(out, "%s is up to date\n", rel(path))
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// writeSkill resolves where a named skill goes, then writes or checks it.
func (c Command) writeSkill(stdout io.Writer, name, body string, check bool) error {
	root, err := root(".")
	if err != nil {
		return err
	}
	return writeSkillTo(stdout, c.Name, skillPaths(root, name), body, check)
}

// skillPaths are the three copies of a manual for name: the one the release
// ships, and the ones each agent reads in this repo.
func skillPaths(root, name string) []string {
	paths := []string{filepath.Join(root, ShippedDir, name, SkillFile)}
	for _, dir := range AgentDirs() {
		paths = append(paths, filepath.Join(root, dir, name, SkillFile))
	}
	return paths
}

// version is `<Name> version`, and with --pin the mise.toml line that
// installs exactly this build.
//
// The version and the key are what the binary was made with, so the line is
// right by construction: it cannot name a version that was never released or
// a key that no longer signs, which is the failure a hand-written README has
// and cannot detect.
func (c Command) version(call Call) error {
	if !call.Given("pin") {
		fmt.Fprintln(call.Stdout, c.Version)
		return nil
	}
	if c.Pin == "" || c.PubKey == "" {
		return fmt.Errorf("%s does not say how it is pinned; it ships some other way", c.Name)
	}
	fmt.Fprintf(call.Stdout, "%q = { version = %q, pubkey = %q }\n",
		"packslip:"+c.Pin, strings.TrimPrefix(c.Version, "v"), c.PubKey)
	return nil
}

// pinFlag is `version --pin`.
func pinFlag(fs *flag.FlagSet) {
	fs.Var(new(Bool), "pin", "print the mise.toml line that installs this build")
}

// checkFlag is `skill --check`: say whether every copy is current, write none.
func checkFlag(fs *flag.FlagSet) {
	fs.Var(new(Bool), "check", "say whether each copy is current; write nothing")
}

// PinMarker is the line a README puts where the install block belongs. The
// block between two of them is written by `<cmd> skill`, from the same
// values `version --pin` prints, so the one instruction a reader follows
// before they have the tool is generated by the tool.
//
// A README that writes the key out by hand goes stale the day the key
// rotates and nothing tells it, which is how the snippet in this one came to
// name no key at all and install nothing.
const PinMarker = "<!-- pin -->"

// readme rewrites the block between the two markers, or with check says it is
// stale. A README without the marker is left alone: not every command has
// one, and none is required to.
func (c Command) readme(out io.Writer, dir string, check bool) error {
	if c.Pin == "" || c.PubKey == "" {
		return nil
	}
	path := filepath.Join(dir, "README.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	before, rest, found := strings.Cut(string(data), PinMarker)
	if !found {
		return nil
	}
	_, after, found := strings.Cut(rest, PinMarker)
	if !found {
		return fmt.Errorf("%s has one %s and needs two, around the block to write", rel(path), PinMarker)
	}
	want := before + PinMarker + "\n" + c.pinBlock() + PinMarker + after
	written, err := put(out, c.Name, path, want, check)
	if err != nil {
		return err
	}
	if written {
		fmt.Fprintf(out, "wrote the install block in %s\n", rel(path))
	}
	return nil
}

// pinBlock is what a consumer pastes. The version is left as a placeholder on
// purpose: the key is the part that goes stale silently, and the version is
// whatever the Releases page says today.
func (c Command) pinBlock() string {
	return fmt.Sprintf("\n```toml\n[tools]\n%q = { version = \"<version>\", pubkey = %q }\n```\n\n"+
		"The Releases page has the latest version, and `%s version --pin` prints\nthis line with the version of the build you already have. This block is\nwritten by `%s skill` from the key the releases are signed with, so it\ncannot name a key that no longer signs.\n\n",
		"packslip:"+c.Pin, c.PubKey, c.Name, c.Name)
}
