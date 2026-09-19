// Package session keeps a repo's Claude Code skills pinned, and keeps the
// rest of the repo's Claude Code session from being decided somewhere else.
//
// What is pinned lives in session.toml; this package only reads it. It runs as
// `dev session ...`, so every developer workflow stays in one
// binary.
package session

import (
	_ "embed"
	"flag"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/conf"
)

// The binaries session shells out to: claude runs the session checks,
// git reads remotes and upstream refs, ps and lsof find live sessions.
const (
	ClaudeBin = "claude"
	// claudePin is what installs it, quoted when it is missing. Claude Code
	// is installed by its own installer rather than by mise, so this names
	// that instead of a [tools] line.
	claudePin = "install Claude Code: https://claude.com/product/claude-code"
	GitBin    = "git"
	PsBin     = "ps"
	LsofBin   = "lsof"
)

// syncCmd is how this repo spells "run sync", quoted back in every error that
// a sync would fix. session.toml sets it to the mise task; the default covers
// a repo that runs the dev binary directly.
var syncCmd = "mise run session:sync"

// Usage is what dev prints for these verbs. It is markdown in a file beside
// this one, not a string const: a Go raw string is backtick-delimited, so it
// can never hold the inline code that keeps a `<placeholder>` from reaching a
// markdown renderer as an HTML tag.

//go:embed usage.md
var Usage string

// Subs are session's subcommands, each declaring what it takes the way every
// other verb on the stack does: cli parses the flags and the positionals, so
// this package no longer carries a switch over args[0], a requireNoArgs, or a
// hand-rolled reader for one bool flag. That trio was the pre-Call shape, and
// it is why session sat out of the verb table while every other package moved.
var Subs = map[string]cli.Verb{
	"sync":   {Run: withPins(func(c cli.Call) error { return Sync(c.Stdout) }), Desc: "write .claude/skills and the .claude/settings.json keys session.toml implies"},
	"check":  {Run: withPins(Check), Flags: cli.ReportFlags, Desc: "fail when either has drifted from session.toml"},
	"verify": {Run: withPins(runVerify), Flags: VerifyFlags, Desc: "hold a fresh Claude Code session against SESSION.lock"},
	"bump":   {Run: withPins(runBump), Args: "[SOURCE...]", Desc: "move a pin in session.toml to upstream HEAD"},
	"mcp":    {Run: withPins(func(c cli.Call) error { return MCP(c.Stdout, c.Stderr) }), Desc: "every MCP server .mcp.json declares connects"},
}

// VerifyFlags is verify's only flag. cli.Bool takes "" as false, so a mise
// task may pass `--update=$usage_update` with the variable unset — which is
// the whole reason this package once read the flag by hand.
func VerifyFlags(fs *flag.FlagSet) {
	fs.Var(new(cli.Bool), "update", "record this session in SESSION.lock instead of only reporting")
}

func runVerify(c cli.Call) error { return Verify(c.Stdout, c.Given("update")) }

func runBump(c cli.Call) error { return Bump(c.Stdout, c.Args) }

// withPins reads session.toml's sync_command before the subcommand runs, so
// every error that a sync would fix names this repo's own way of running one.
// A wrapper rather than a line at the top of five functions: it is one fact,
// and five copies of it drift the moment a sixth subcommand is written.
func withPins(run cli.Runner) cli.Runner {
	return func(c cli.Call) error {
		applySyncCommand()
		return run(c)
	}
}

// applySyncCommand takes sync_command from session.toml before anything runs,
// so that commands which never load the pins -- verify reads only the lock --
// still name this repo's own way of running a sync. A broken or missing file
// is not this function's business; whatever runs next reports it properly.
func applySyncCommand() {
	config, err := conf.Load[struct {
		SyncCommand string `toml:"sync_command"`
	}](pinsFile)
	if err == nil && config.SyncCommand != "" {
		syncCmd = config.SyncCommand
	}
}
