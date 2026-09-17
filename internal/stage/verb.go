// The stage verbs: what each takes and what it runs. What they do lives in
// stage.go, and what a directory holds in inspect.go.
// Package stage builds, runs and checks one command directory from what it
// finds there: a Go main, a package.json (Vite), gsx sources, a wrangler.toml
// (a Worker, one wasm per environment), a fly.toml (a Fly app). mise names a
// stage per directory;
// this does the rest, so a new command is new lines in mise.toml, not new
// tooling.
package stage

import (
	_ "embed"
	"flag"
	"io"

	"github.com/joeblew999/dev/cli"
)

//go:embed usage.md
var Usage string

// EnvFlag is the wrangler environment, taken by wasm and workerd.
func EnvFlag(fs *flag.FlagSet) {
	fs.String("env", "", "wrangler environment `NAME`")
}

// CheckFlags are what `check` requests and expects of the smoke check.
func CheckFlags(fs *flag.FlagSet) {
	fs.String("path", "/", "the path `P` the smoke check and the browser probe request")
	fs.String("expect", "", "the `TEXT` the smoke check's body must contain")
}

// Run is every stage verb. DIR comes first; flags may follow anywhere.
// Each verb is its own function. One function switching on the verb it was
// called as means the verb names live here as well as in the table that
// declares them, and two lists of the same five strings drift.

// BuildVerb is `build`: the binary, and the manual when the command has verbs.
func BuildVerb(verb string, args []string, stdout, stderr io.Writer) error {
	dir, err := dirOnly(verb, args, stderr, nil)
	if err != nil {
		return err
	}
	return Build(stdout, dir, false, "")
}

// WasmVerb is `wasm`: the same command built as a Worker.
func WasmVerb(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	EnvFlag(fs)
	dir, _, err := cli.DirAnd(fs, args, 0)
	if err != nil {
		return err
	}
	return Build(stdout, dir, true, cli.Value(fs, "env"))
}

// CheckVerb is `check`: everything that says the command is sound.
func CheckVerb(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	CheckFlags(fs)
	dir, _, err := cli.DirAnd(fs, args, 0)
	if err != nil {
		return err
	}
	return Check(stdout, dir, cli.Value(fs, "path"), cli.Value(fs, "expect"))
}

// RunVerb is `run`: the built binary, under the repo's secrets.
func RunVerb(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	dir, rest, err := cli.DirAnd(fs, args, -1)
	if err != nil {
		return err
	}
	return Exec(dir, false, "", rest)
}

// WorkerdVerb is `workerd`: the Worker on local workerd.
func WorkerdVerb(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	EnvFlag(fs)
	dir, rest, err := cli.DirAnd(fs, args, -1)
	if err != nil {
		return err
	}
	return Exec(dir, true, cli.Value(fs, "env"), rest)
}

// dirOnly is a verb that takes a directory and nothing else.
func dirOnly(verb string, args []string, stderr io.Writer, flags func(*flag.FlagSet)) (string, error) {
	fs := cli.Flags(verb, stderr)
	if flags != nil {
		flags(fs)
	}
	dir, _, err := cli.DirAnd(fs, args, 0)
	return dir, err
}
