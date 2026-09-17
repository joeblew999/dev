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
func Run(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	switch verb {
	case "build":
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		return Build(stdout, dir, false, "")
	case "wasm":
		EnvFlag(fs)
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		return Build(stdout, dir, true, cli.Value(fs, "env"))
	case "check":
		CheckFlags(fs)
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		return Check(stdout, dir, cli.Value(fs, "path"), cli.Value(fs, "expect"))
	case "run":
		dir, rest, err := cli.DirAnd(fs, args, -1)
		if err != nil {
			return err
		}
		return Exec(dir, false, "", rest)
	case "workerd":
		EnvFlag(fs)
		dir, rest, err := cli.DirAnd(fs, args, -1)
		if err != nil {
			return err
		}
		return Exec(dir, true, cli.Value(fs, "env"), rest)
	}
	return cli.Usagef("stage: unknown verb %q", verb)
}
