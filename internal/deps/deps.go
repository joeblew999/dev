// Package deps runs go-mod-upgrade in every Go module of the repo, so a
// dependency bump never misses a nested module. It runs as `dev deps ...`.
package deps

import (
	_ "embed"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/joeblew999/dev/cli"
)

// Usage is what dev prints for these verbs. It is markdown in a file beside
// this one, not a string const: a Go raw string is backtick-delimited, so it
// can never hold the inline code that keeps a `<placeholder>` from reaching a
// markdown renderer as an HTML tag.

//go:embed usage.md
var Usage string

// Run is `dev deps list|upgrade`.
// Subs are deps' subcommands. Neither takes a flag, so each declares only
// that it takes none; cli renders `dev deps list` from the names alone.
var Subs = map[string]cli.Verb{
	"list":    {Run: list, Desc: "show which Go dependencies have newer versions, across every module"},
	"upgrade": {Run: upgrade, Desc: "walk through those upgrades and pick the ones you want"},
}

// list and upgrade are the two subcommands. Each is its own function rather
// than one that reads the verb it was called as, so a subcommand's name is in
// Subs and nowhere else at all.
func list(c cli.Call) error    { return each(c, "--list") }
func upgrade(c cli.Call) error { return each(c) }

// each runs go-mod-upgrade in every module of the repo. Listing and upgrading
// are the same walk; --list is the whole difference.
func each(c cli.Call, flags ...string) error {
	if len(c.Args) > 0 {
		return c.Usagef("takes no arguments")
	}
	dirs, err := Modules(".")
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		fmt.Fprintf(c.Stdout, "== %s ==\n", dir)
		cmd := exec.Command("go-mod-upgrade", flags...)
		cmd.Dir = dir
		cmd.Stdin, cmd.Stdout, cmd.Stderr = c.Stdin, c.Stdout, c.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: go-mod-upgrade: %w; pin it in mise.toml: \"go:github.com/oligot/go-mod-upgrade\" = \"latest\"", dir, err)
		}
	}
	return nil
}

// skipped are directories that never hold a module of ours.
var skipped = map[string]bool{".git": true, "node_modules": true, "vendor": true, ".dist": true, "dist": true, "build": true, ".bin": true, "bin": true}

// Modules lists every directory under root holding a go.mod, root first.
func Modules(root string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != root && skipped[d.Name()] {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "go.mod" {
			dirs = append(dirs, filepath.Dir(p))
		}
		return nil
	})
	sort.Strings(dirs)
	return dirs, err
}
