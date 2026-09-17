// Package scaffold is `dev init`: the stack's files written into a new repo,
// from the templates this binary carries. Each is the file the reference
// repo proved, with the repo's name and this tool's version filled in. It
// refuses to touch a file that exists and names it, so it is safe to run in a
// repo that has some of the stack already.
package scaffold

import (
	"embed"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/gitrepo"
)

//go:embed all:files
var files embed.FS

// Usage is what dev prints for these verbs. It is markdown in a file beside
// this one, not a string const: a Go raw string is backtick-delimited, so it
// can never hold the inline code that keeps a `<placeholder>` from reaching a
// markdown renderer as an HTML tag.

//go:embed usage.md
var Usage string

// Run is `dev init`.
func Run(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	name := fs.String("name", "", "the first command's name (default: the directory's)")
	pin := fs.String("pin", "", "the dev release to pin (default: this binary's version)")
	pubkey := fs.String("pubkey", "", "the public key its releases are signed with (default: this binary's)")
	var dir string
	var err error
	// DIR defaults to "." when omitted (no args, or flags first); when
	// given it comes first and flags may follow anywhere.
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		dir, _, err = cli.DirAnd(fs, append([]string{"."}, args...), 0)
	} else {
		dir, _, err = cli.DirAnd(fs, args, 0)
	}
	if err != nil {
		return err
	}
	return Init(stdout, dir, *name, *pin, *pubkey)
}

// Version is this binary's release, "dev" when built by hand, and Pubkey the
// public key its releases are signed with, "" when none; main sets both.
var (
	Version = "dev"
	Pubkey  = ""
)

var validName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Init writes the stack into dir.
func Init(out io.Writer, dir, name, pin, pubkey string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if name == "" {
		name = filepath.Base(abs)
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("%q cannot name a command: use lowercase letters, digits and dashes (--name)", name)
	}
	if pin == "" {
		pin = Version
	}
	if pin == "dev" {
		return fmt.Errorf("this dev was built by hand and has no release to pin; pass --pin VERSION")
	}
	if pubkey == "" {
		pubkey = Pubkey
	}
	devPin := fmt.Sprintf("{ version = %q, pubkey = %q }", pin, pubkey)
	if pubkey == "" {
		devPin = fmt.Sprintf("%q", pin)
	}
	slug := slugOf(abs)
	module := "github.com/" + slug
	if slug == "" {
		slug = "<owner>/" + name
		module = "example.com/" + name
	}
	replace := strings.NewReplacer("__NAME__", name, "__DEV__", devPin, "__PIN__", pin, "__MODULE__", module, "__SLUG__", slug)

	// An existing repo keeps its shape: one with a module at the root gets no
	// go.work and no nested module, and a command that exists is not
	// rewritten. A new repo gets the first command and the workspace.
	rootModule := exists(filepath.Join(dir, "go.mod"))
	cmdExists := exists(filepath.Join(dir, "cmd", name))
	skip := func(rel string) string {
		switch {
		case rootModule && rel == "go.work":
			return "a module at the root needs no go.work"
		case rootModule && strings.HasSuffix(rel, "/go.mod"):
			return "a module at the root holds the command"
		case cmdExists && strings.HasPrefix(rel, "cmd/"):
			return "the command exists"
		}
		return ""
	}
	var written, kept, skipped []string
	err = fs.WalkDir(files, "files", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(path, "files/"), ".tmpl")
		if why := skip(replace.Replace(rel)); why != "" {
			skipped = append(skipped, replace.Replace(rel)+" ("+why+")")
			return nil
		}
		target := filepath.Join(dir, filepath.FromSlash(replace.Replace(rel)))
		if _, err := os.Stat(target); err == nil {
			kept = append(kept, target)
			return nil
		}
		data, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		if rel == "mise.toml" {
			data = []byte(currentPins(string(data)))
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.Contains(rel, "/hooks/") {
			mode = 0o755
		}
		if err := os.WriteFile(target, []byte(replace.Replace(string(data))), mode); err != nil {
			return err
		}
		written = append(written, target)
		return nil
	})
	if err != nil {
		return err
	}
	for _, w := range written {
		fmt.Fprintln(out, "wrote", w)
	}
	for _, k := range kept {
		fmt.Fprintln(out, "kept ", k, "(exists; compare it with what dev init would write)")
	}
	for _, s := range skipped {
		fmt.Fprintln(out, "not written:", s)
	}
	if len(written) == 0 {
		return fmt.Errorf("nothing written: every file exists already")
	}
	// The command's module requires the pinned dev; resolve it now so the
	// first build needs no step, and run its own `skill` verb once so both
	// copies of its manual exist from the first commit. An existing command
	// is kept as it is; its builds keep its manual current. Offline, or
	// before that release exists, say what will.
	if cmdDir := filepath.Join(dir, "cmd", name); !cmdExists && exists(filepath.Join(cmdDir, "go.mod")) {
		if !exists(filepath.Join(cmdDir, "go.sum")) {
			if err := tidy(cmdDir); err != nil {
				fmt.Fprintf(out, "not resolved: %v\n  once dev %s is published: cd %s && go mod tidy\n", err, pin, cmdDir)
			}
		}
		switch {
		case !exists(filepath.Join(cmdDir, "go.sum")):
			fmt.Fprintf(out, "not generated: the module is unresolved\n  once dev %s is published: cd %s && go mod tidy && go run . skill\n", pin, cmdDir)
		default:
			if err := generateSkill(cmdDir); err != nil {
				fmt.Fprintf(out, "not generated: %v\n  repair it with: cd %s && go run . skill\n", err, cmdDir)
			}
		}
	}
	fmt.Fprintf(out, "\n%s is on the stack, pinned to dev %s, module %s.\nNext, in %s:\n  mise trust && mise install && mise run test\n", name, pin, module, dir)
	return nil
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// pinLine is an exact packslip pin in the template mise.toml.
var pinLine = regexp.MustCompile(`(?m)^("packslip:[^"]+") = "(\d+\.\d+\.\d+)"`)

// currentPins moves every exact packslip pin in the template to the release
// mise knows today, so a new repo starts current, not at whatever the
// template said when this dev was built. The dev pin itself is this binary's
// version and is left alone. A pin mise cannot resolve keeps the template's.
func currentPins(mise string) string {
	return pinLine.ReplaceAllStringFunc(mise, func(line string) string {
		m := pinLine.FindStringSubmatch(line)
		tool := strings.Trim(m[1], `"`)
		if strings.Contains(tool, "joeblew999/dev") {
			return line
		}
		if v := latest(tool); v != "" {
			return m[1] + ` = "` + v + `"`
		}
		return line
	})
}

// The binaries init shells out to: go resolves the new module and runs
// its skill verb, mise reports the current pins.
const (
	GoBin   = "go"
	MiseBin = "mise"
)

// tidy resolves a new command's module: the dev release it requires. A
// variable so tests can replace it.
var tidy = func(dir string) error {
	cmd := exec.Command(GoBin, "mod", "tidy")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// generateSkill runs a new command's own `skill` verb once, so every copy
// of its manual exist from the first commit: the first `dev check` is green,
// the first release ships a skill, and the first Claude Code session in the
// repo has it in context. A variable so tests can replace it.
var generateSkill = func(dir string) error {
	cmd := exec.Command(GoBin, "run", ".", "skill")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go run . skill: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// latest asks mise for a tool's newest release, "" when it cannot say. A
// variable so tests can replace it.
var latest = func(tool string) string {
	out, err := exec.Command(MiseBin, "latest", tool).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// slugOf is owner/repo from dir's git remote, "" without one.
func slugOf(dir string) string {
	slug, err := gitrepo.Slug(dir)
	if err != nil {
		return ""
	}
	return slug
}
