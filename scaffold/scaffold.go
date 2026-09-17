// Package scaffold is `dev init`: the stack's files written into a new repo,
// from the templates this binary carries. Each is the file the reference
// repo proved, with the repo's name and this tool's version filled in. It
// refuses to touch a file that exists and names it, so it is safe to run in a
// repo that has some of the stack already.
package scaffold

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joeblew999/dev/internal/cli"
)

//go:embed all:files
var files embed.FS

const Usage = `dev init [DIR] [--name NAME] [--pin VERSION]
    write the stack into DIR (default .): mise.toml with the tools pinned and
    the stack's tasks, hk.pkl, session.toml, .mcp.json, the Claude Code
    settings and skill hook, the two workflows, .gitignore, AGENTS.md, and a
    first command cmd/NAME (an HTTP server answering /health) with its module
    and go.work. NAME defaults to DIR's name; the module path comes from the
    git remote, or example.com without one. VERSION is the dev release to
    pin; default this binary's own. Existing files are left alone and named.
    Then: mise trust && mise install && mise run test
`

// Run is `dev init`.
func Run(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	name := fs.String("name", "", "the first command's name (default: the directory's)")
	pin := fs.String("pin", "", "the dev release to pin (default: this binary's version)")
	dir, _, err := cli.DirAnd(fs, append([]string{"."}, args...), 0)
	if err != nil {
		return err
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		dir = args[0]
		if _, _, err := cli.DirAnd(fs, args, 0); err != nil {
			return err
		}
	}
	return Init(stdout, dir, *name, *pin)
}

// Version is this binary's release, set by the build; "dev" when built by hand.
var Version = "dev"

var validName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Init writes the stack into dir.
func Init(out io.Writer, dir, name, pin string) error {
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
	slug := slugOf(abs)
	module := "github.com/" + slug
	if slug == "" {
		slug = "<owner>/" + name
		module = "example.com/" + name
	}
	replace := strings.NewReplacer("__NAME__", name, "__DEV__", pin, "__MODULE__", module, "__SLUG__", slug)

	var written, kept []string
	err = fs.WalkDir(files, "files", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(path, "files/"), ".tmpl")
		target := filepath.Join(dir, filepath.FromSlash(replace.Replace(rel)))
		if _, err := os.Stat(target); err == nil {
			kept = append(kept, target)
			return nil
		}
		data, err := files.ReadFile(path)
		if err != nil {
			return err
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
	if len(written) == 0 {
		return fmt.Errorf("nothing written: every file exists already")
	}
	fmt.Fprintf(out, "\n%s is on the stack, pinned to dev %s, module %s.\nNext, in %s:\n  mise trust && mise install && mise run test\n", name, pin, module, dir)
	return nil
}

// slugOf is owner/repo from dir's git remote, "" without one.
func slugOf(dir string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))
	url = strings.TrimSuffix(url, ".git")
	for _, prefix := range []string{"https://github.com/", "git@github.com:", "ssh://git@github.com/"} {
		if rest, ok := strings.CutPrefix(url, prefix); ok && strings.Count(rest, "/") == 1 {
			return rest
		}
	}
	return ""
}
