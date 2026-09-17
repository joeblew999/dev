// The stage verbs: what each takes and what it runs. What they do lives in
// stage.go, and what a directory holds in inspect.go.
package stage

import (
	_ "embed"
	"flag"

	"github.com/joeblew999/dev/cli"
)

// Usage is the prose for these verbs. It is markdown in a file beside this
// one, not a string const: a Go raw string is backtick-delimited, so it can
// never hold the inline code that keeps `<dir>` from reaching a markdown
// renderer as an HTML tag.

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

// Each verb is the work it does. cli parses what the verb declared — its Args
// and its Flags — so none of them opens with a FlagSet, a DirAnd and an error
// check the way all five used to.

func BuildVerb(c cli.Call) error   { return Build(c.Stdout, c.Dir, false, "") }
func WasmVerb(c cli.Call) error    { return Build(c.Stdout, c.Dir, true, c.Value("env")) }
func RunVerb(c cli.Call) error     { return Exec(c.Dir, false, "", c.Args) }
func WorkerdVerb(c cli.Call) error { return Exec(c.Dir, true, c.Value("env"), c.Args) }

func CheckVerb(c cli.Call) error {
	return Check(c.Stdout, c.Dir, c.Value("path"), c.Value("expect"))
}
