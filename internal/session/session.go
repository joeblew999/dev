package session

import (
	_ "embed"
	"flag"

	"github.com/joeblew999/dev/cli"
)

// The binaries session shells out to: claude runs the session checks,
// git reads remotes and upstream refs, ps and lsof find live sessions.
const (
	ClaudeBin = "claude"
	GitBin    = "git"
	PsBin     = "ps"
	LsofBin   = "lsof"
)

// Usage is what dev prints for these verbs. It is markdown in a file beside
// this one, not a string const: a Go raw string is backtick-delimited, so it
// can never hold the inline code that keeps a `<placeholder>` from reaching a
// markdown renderer as an HTML tag.

//go:embed usage.md
var Usage string

// Subs are session's subcommands, each declaring what it takes the way every
// other verb on the stack does.
//
// They used to be wrapped one by one in a function whose whole job was to read
// session.toml's sync_command into a package variable before the verb ran, so
// that errors would name this repo's own way of running a sync. What an error
// said therefore depended on whether that wrapper had run, and a sixth
// subcommand written without it would have printed a command the repo does not
// have. The spelling is a fact of the repo, so pins reads it from the repo,
// once, when a message first needs it — and the wrapper is gone.
var Subs = map[string]cli.Verb{
	"sync":   {Run: func(c cli.Call) error { return Sync(c.Stdout) }, Desc: "write every agent's skills directory and the .claude/settings.json keys session.toml implies"},
	"check":  {Run: Check, Flags: CheckFlags, Desc: "fail when either has drifted from session.toml, and with --fix put it back"},
	"verify": {Run: runVerify, Flags: VerifyFlags, Desc: "hold a fresh Claude Code session against SESSION.lock"},
	"bump":   {Run: runBump, Args: "[SOURCE...]", Desc: "move a pin in session.toml to upstream HEAD"},
	"remove": {Run: func(c cli.Call) error { return Remove(c.Stdout) }, Desc: "take back every skill sync put here, leaving this repo's own alone"},
	"mcp":    {Run: func(c cli.Call) error { return MCP(c.Stdout, c.Stderr) }, Desc: "every MCP server .mcp.json declares connects"},
}

// VerifyFlags is verify's only flag. cli.Bool takes "" as false, so a mise
// task may pass `--update=$usage_update` with the variable unset — which is
// the whole reason this package once read the flag by hand.
func VerifyFlags(fs *flag.FlagSet) {
	fs.Var(new(cli.Bool), "update", "record this session in SESSION.lock instead of only reporting")
}

func runVerify(c cli.Call) error { return Verify(c.Stdout, c.Given("update")) }

func runBump(c cli.Call) error { return Bump(c.Stdout, c.Args) }
