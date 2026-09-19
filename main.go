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
	"github.com/joeblew999/dev/internal/seo"
	"github.com/joeblew999/dev/internal/session"
	"github.com/joeblew999/dev/internal/stage"
)

// skill.md is the manual a person writes: the prose, with a marker line where
// the rendered verbs go. Beside main.go, because main.go is the command it
// describes — the same rule that puts each package's usage.md beside its
// verbs. Markdown files, not string consts, so prose
// edits stay prose; go:embed compiles them into the binary, so `dev skill`
// works anywhere and the rendered manual ships via release.

//go:embed skill.md
var skillDoc string

// cliSkill is the manual for the library rather than for dev's verbs, so it
// lives in cli/ beside what it documents. main.go only embeds it, because a
// skill ships from the command that declares it.

//go:embed cli/skill.md
var cliSkill string

// dev is the whole tool: what each verb runs, and its usage. cli.Main runs it
// and renders the manual from this table, so the manual is the code's; any
// command built the same way gets the same.
var dev = cli.Command{
	Name: "dev",
	Verbs: map[string]cli.Verb{
		"build":   {Run: stage.BuildVerb, Args: "DIR", Desc: "make the binary, and the manual when the command has verbs", Usage: stage.Usage},
		"wasm":    {Run: stage.WasmVerb, Args: "DIR", Flags: stage.EnvFlag, Desc: "build the same command as a Worker instead, for the environment you name", Usage: stage.Usage},
		"check":   {Run: stage.CheckVerb, Args: "DIR", Flags: stage.CheckFlags, Desc: "everything that says the command is sound; this is what CI runs", Usage: stage.Usage},
		"run":     {Run: stage.RunVerb, Args: "DIR [-- ARGS]", Desc: "run the built binary with the repo's secrets loaded", Usage: stage.Usage},
		"workerd": {Run: stage.WorkerdVerb, Args: "DIR [-- ARGS]", Flags: stage.EnvFlag, Desc: "serve the Worker locally, the way Cloudflare will run it", Usage: stage.Usage},
		"deploy":  {Run: app.DeployVerb, Args: "DIR [-- FLAGS]", Flags: app.DeployFlags, Desc: "put the command in the cloud its directory names", Usage: app.Usage},
		"url":     {Run: app.URLVerb, Args: "DIR", Flags: app.URLFlags, Desc: "print the address to talk to, deployed or local", Usage: app.Usage},
		"logs":    {Run: app.LogsVerb, Args: "DIR", Flags: app.EnvFlag, Desc: "follow the deployed app's logs as they happen", Usage: app.Usage},
		"smoke":   {Run: app.SmokeVerb, Args: "DIR", Flags: app.SmokeFlags, Desc: "start the Worker locally and make one request, to know a build is not broken", Usage: app.Usage},
		"wait":    {Run: app.WaitVerb, Args: "URL", Flags: app.WaitFlags, Desc: "poll a URL until it answers steadily", Usage: app.Usage},
		"seo":     {Subs: seo.Subs, Usage: seo.Usage},
		"delete":  {Run: app.DeleteVerb, Args: "DIR", Flags: app.DeleteFlags, Desc: "remove a deployed app, and the storage created with it; asks first", Usage: app.Usage},
		"secrets": {Subs: secrets.Subs, Usage: secrets.Usage},
		"session": {Subs: session.Subs, Usage: session.Usage},
		// session was out of this table while it still answered to the
		// pre-Call signature: the manual, the index and --help are all
		// rendered from here, so a verb left out is gone from every one of
		// them with nothing else to change. It declares its subcommands like
		// every other package now, so it is back.
		"release": {Run: release.Run, Args: "DIR [VERSION]", Flags: release.Flags, Desc: "build for every platform, sign it, and publish it to GitHub", Usage: release.Usage},
		"deps":    {Subs: deps.Subs, Usage: deps.Usage},
	},
	Skill: skillDoc,

	Skills: map[string]string{"cli": cliSkill},

	// The manual's reading order: start a repo, build it, ship it, then the
	// verbs that keep it. Without this the sections fall in verb-name order,
	// which puts init fifth — the first thing anyone does, halfway down.
	Order: []string{"build", "deploy", "secrets", "release", "deps", "skill"},
}

// version is set by the release build (-X main.version).
var version = "dev"

func main() {
	dev.Version = version
	cli.Main(dev)
}
