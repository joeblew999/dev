// Command dev is the stack's developer tool. It is not part of any binary that
// ships: mise tasks build and run it. A task names a stage of a command
// directory; dev reads the directory and does the rest, so a new command is
// new lines in mise.toml, never new tooling. Every task, test and release
// included, runs the same on a developer's machine and in GitHub Actions:
// mise.toml is the one source of truth for both, local is the fast path, CI
// proves a machine nobody set up. Every package is one thing, named as the
// tasks name it, and every verb has the one shape in cli.
package main

import (
	_ "embed"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/app"
	"github.com/joeblew999/dev/internal/deps"
	"github.com/joeblew999/dev/internal/release"
	"github.com/joeblew999/dev/internal/secrets"
	"github.com/joeblew999/dev/internal/session"
	"github.com/joeblew999/dev/internal/stage"
)

// skill holds the prose around the verbs in the generated manual: head.md
// before them, tail.md after. Markdown files, not string consts, so prose
// edits stay prose; go:embed compiles them into the binary, so `dev skill`
// works anywhere and the rendered manual ships via release.

//go:embed skill/head.md
var skillHead string

//go:embed skill/tail.md
var skillTail string

// cliSkill is the manual for the library rather than for dev's verbs. A repo
// is asked to write its commands against cli, so it needs the library's API
// and rules; before this they were prose two thirds of the way down dev's own
// manual, and the API was in no manual at all.

//go:embed skill/cli.md
var cliSkill string

// dev is the whole tool: what each verb runs, and its usage. cli.Main runs it
// and renders the manual from this table, so the manual is the code's; any
// command built the same way gets the same.
var dev = cli.Command{
	Name: "dev",
	Verbs: map[string]cli.Verb{
		"build":   {Run: stage.Run, Usage: stage.Usage},
		"wasm":    {Run: stage.Run, Usage: stage.Usage},
		"check":   {Run: stage.Run, Usage: stage.Usage},
		"run":     {Run: stage.Run, Usage: stage.Usage},
		"workerd": {Run: stage.Run, Usage: stage.Usage},
		"deploy":  {Run: app.Run, Usage: app.Usage},
		"url":     {Run: app.Run, Usage: app.Usage},
		"logs":    {Run: app.Run, Usage: app.Usage},
		"smoke":   {Run: app.Run, Usage: app.Usage},
		"wait":    {Run: app.Run, Usage: app.Usage},
		"delete":  {Run: app.Run, Usage: app.Usage},
		"secrets": {Run: secrets.Run, Usage: secrets.Usage},
		"session": {Run: session.Run, Usage: session.Usage},
		"release": {Run: release.Run, Usage: release.Usage},
		"deps":    {Run: deps.Run, Usage: deps.Usage},
	},
	Head: skillHead,
	Tail: skillTail,

	Skills: map[string]string{"cli": cliSkill},

	// The manual's reading order: start a repo, build it, ship it, then the
	// verbs that keep it. Without this the sections fall in verb-name order,
	// which puts init fifth — the first thing anyone does, halfway down.
	Order: []string{"build", "deploy", "secrets", "release", "deps", "session", "skill"},
}

// version is set by the release build (-X main.version).
var version = "dev"

func main() {
	dev.Version = version
	cli.Main(dev)
}
