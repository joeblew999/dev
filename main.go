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
	"github.com/joeblew999/dev/app"
	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/deps"
	"github.com/joeblew999/dev/release"
	"github.com/joeblew999/dev/scaffold"
	"github.com/joeblew999/dev/secrets"
	"github.com/joeblew999/dev/session"
	"github.com/joeblew999/dev/stage"
)

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
		"init":    {Run: scaffold.Run, Usage: scaffold.Usage},
	},
	Head: skillHead,
	Tail: skillTail,
}

// version and pubkey are set by the release build (-X main.version, -X
// main.pubkey): the release's version, and the public key it is signed with.
var (
	version = "dev"
	pubkey  = ""
)

func main() {
	scaffold.Version, scaffold.Pubkey = version, pubkey
	dev.Version = version
	cli.Main(dev)
}

const skillHead = `---
name: dev
description: Build, check, run, release and deploy the commands of a repo on the mise + fnox + hk + packslip stack. Use before running go, npm, wrangler, fly, fnox or goreleaser by hand in such a repo: a mise task names a stage of a command directory and dev does the rest. Tests and releases run the same locally and in GitHub Actions, from the same tasks.
---

# dev

A repo on this stack is a few commands, each its own directory and Go module:
main.go, and beside it a worker.go and wrangler.toml if it deploys to
Cloudflare, a fly.toml if it deploys to Fly, a package.json and .gsx sources if
it has a UI. A repo that is one command keeps it at the root and uses ` + "`.`" + `.
A new repo gets the whole stack from ` + "`dev init`" + `.
Every command has the same stages, and a mise task names one:
` + "`<cmd>:<stage>[:variant]`" + `, so ` + "`mise run proxy:deploy`" + ` runs
` + "`dev deploy cmd/proxy`" + `. Run stages through their tasks (` + "`mise tasks`" + `
lists them), never by hand, and never call go, npm, wrangler, fly or fnox
directly when a task exists. Anything typed after a task name passes to the
command.

## Verbs

The directory a verb acts on comes first; flags may follow anywhere, and
everything after a bare ` + "`--`" + ` goes to the program being run.

`

const skillTail = `## What a repo supplies

- ` + "`[vars] worker`" + ` in mise.toml: the command whose secrets ` + "`secrets:*`" + ` manage.
- A ` + "`check`" + ` task, what ` + "`mise run test`" + ` runs after the stack's own checks.
- A ` + "`validate`" + ` task, what ` + "`deploy`" + ` runs first.
- A ` + "`secrets:list`" + ` task printing NAME<TAB>OWNER lines, what ` + "`secrets:*`" + ` work from.

## Rules the tool keeps

- Nothing personal in a committed file. Cloud credentials come from fnox; a
  Worker's provisioned ids never reach git (deploy runs on a throwaway copy of
  wrangler.toml); the account's workers.dev subdomain and a developer's
  DEPLOY_SUFFIX live in gitignored mise.local.toml.
- Secret values only ever pass through fnox and the deploy CLI, never an argument.
- Every error names its fix.
- Every task runs the same locally and in GitHub Actions: mise run test and
  mise run release are what CI runs, from the one mise.toml. Local is the fast
  path day to day; CI proves a machine nobody set up. Neither replaces the other.
`
