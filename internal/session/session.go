// Package session keeps a repo's Claude Code skills pinned, and keeps the
// rest of the repo's Claude Code session from being decided somewhere else.
//
// What is pinned lives in session.toml; this package only reads it. It runs as
// `dev session ...`, so every developer workflow stays in one
// binary.
package session

import (
	_ "embed"
	"io"
	"strings"

	"github.com/BurntSushi/toml"

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

// Run is `dev session sync|check|verify|bump|mcp`.
func Run(verb string, args []string, stdout, stderr io.Writer) error {
	if cli.HelpRequested(args) {
		return cli.ErrHelp
	}
	applySyncCommand()
	if len(args) == 0 {
		return cli.Usagef("session: sync, check, verify, bump or mcp")
	}
	switch args[0] {
	case "sync":
		if err := requireNoArgs(args); err != nil {
			return err
		}
		return Sync(stdout)
	case "check":
		if err := requireNoArgs(args); err != nil {
			return err
		}
		return Check(stdout)
	case "verify":
		update, ok := updateFlag(args[1:])
		if !ok {
			return cli.Usagef("session verify takes only --update")
		}
		return Verify(stdout, update)
	case "bump":
		return Bump(stdout, args[1:])
	case "mcp":
		if err := requireNoArgs(args); err != nil {
			return err
		}
		return MCP(stdout, stderr)
	}
	return cli.Usagef("session: unknown subcommand %q", args[0])
}

// applySyncCommand takes sync_command from session.toml before anything runs,
// so that commands which never load the pins -- verify reads only the lock --
// still name this repo's own way of running a sync. A broken or missing file
// is not this function's business; whatever runs next reports it properly.
func applySyncCommand() {
	var config struct {
		SyncCommand string `toml:"sync_command"`
	}
	if _, err := toml.DecodeFile(pinsFile, &config); err == nil && config.SyncCommand != "" {
		syncCmd = config.SyncCommand
	}
}

func requireNoArgs(args []string) error {
	if len(args) != 1 {
		return cli.Usagef("session %s takes no arguments", args[0])
	}
	return nil
}

// updateFlag reads verify's only flag. A mise task passes
// `--update=$usage_update`, which is `--update=` when nobody gave the flag.
func updateFlag(args []string) (update, ok bool) {
	if len(args) == 0 {
		return false, true
	}
	if len(args) != 1 {
		return false, false
	}
	name, value, given := strings.Cut(args[0], "=")
	if name != "--update" {
		return false, false
	}
	if !given {
		return true, true // a bare --update means yes
	}
	var b cli.Bool
	if err := b.Set(value); err != nil {
		return false, false
	}
	return bool(b), true
}
