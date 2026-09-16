// Command dev is the stack's developer tool. It is not part of any binary that
// ships: mise tasks build and run it. A task names a stage of a command
// directory; dev reads the directory and does the rest, so a new command is
// new lines in mise.toml, never new tooling. Every package is one thing, named
// as the tasks name it, and every verb has the one shape in internal/cli.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joeblew999/dev/app"
	"github.com/joeblew999/dev/deps"
	"github.com/joeblew999/dev/internal/cli"
	"github.com/joeblew999/dev/release"
	"github.com/joeblew999/dev/secrets"
	"github.com/joeblew999/dev/session"
	"github.com/joeblew999/dev/stage"
)

// verbs is the whole tool: what each verb runs, and its usage. `dev skill`
// renders the skill from this table, so the manual is the code's.
var verbs = map[string]struct {
	run   cli.Runner
	usage string
}{
	"build":   {stage.Run, stage.Usage},
	"wasm":    {stage.Run, stage.Usage},
	"check":   {stage.Run, stage.Usage},
	"run":     {stage.Run, stage.Usage},
	"workerd": {stage.Run, stage.Usage},
	"deploy":  {app.Run, app.Usage},
	"url":     {app.Run, app.Usage},
	"logs":    {app.Run, app.Usage},
	"smoke":   {app.Run, app.Usage},
	"wait":    {app.Run, app.Usage},
	"secrets": {secrets.Run, secrets.Usage},
	"session": {session.Run, session.Usage},
	"release": {release.Run, release.Usage},
	"deps":    {deps.Run, deps.Usage},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, index())
		os.Exit(2)
	}
	verb, args := os.Args[1], os.Args[2:]
	if verb == "skill" {
		if err := skill(args); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	v, ok := verbs[verb]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown verb %q\n\n%s", verb, index())
		os.Exit(2)
	}
	err := v.run(verb, args, os.Stdout, os.Stderr)
	var uerr *cli.UsageError
	if errors.As(err, &uerr) {
		fmt.Fprintf(os.Stderr, "error: %v\n\n%s", err, v.usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// index lists every verb, in the order the tool's packages are listed above.
func index() string {
	seen := map[string]bool{}
	var b strings.Builder
	b.WriteString("dev: the stack's developer tool (run through mise). Verbs, by what does them:\n\n")
	var names []string
	for name := range verbs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		u := verbs[name].usage
		if seen[u] {
			continue
		}
		seen[u] = true
		b.WriteString(u)
		b.WriteString("\n")
	}
	return b.String()
}

// skill renders the tool's Claude Code skill from the verbs' own usage, so
// what an AI reads can never drift from what the binary does. The release
// ships it, and mise links it into every repo that pins the tool.
func skill(args []string) error {
	out, check := "skills/dev/SKILL.md", false
	for _, a := range args {
		switch {
		case a == "--check":
			check = true
		case strings.HasPrefix(a, "--out="):
			out = strings.TrimPrefix(a, "--out=")
		default:
			return fmt.Errorf("dev skill [--out=FILE] [--check]")
		}
	}
	var b strings.Builder
	b.WriteString(skillHead)
	seen := map[string]bool{}
	var names []string
	for name := range verbs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		u := verbs[name].usage
		if seen[u] {
			continue
		}
		seen[u] = true
		b.WriteString("```\n" + u + "```\n\n")
	}
	b.WriteString(skillTail)
	if check {
		have, err := os.ReadFile(out)
		if err != nil || string(have) != b.String() {
			return fmt.Errorf("%s is stale; regenerate it with: dev skill", out)
		}
		fmt.Printf("%s is up to date\n", out)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s from the verbs' own usage\n", out)
	return nil
}

const skillHead = `---
name: dev
description: Build, check, run and deploy the commands of a repo on the mise + fnox + hk + packslip stack. Use before running go, npm, wrangler, fly, fnox or goreleaser by hand in such a repo: a mise task names a stage of a command directory and dev does the rest.
---

# dev

A repo on this stack is a few commands, each its own directory and Go module:
main.go, and beside it a worker.go and wrangler.toml if it deploys to
Cloudflare, a fly.toml if it deploys to Fly, a package.json and .gsx sources if
it has a UI. A repo that is one command keeps it at the root and uses ` + "`.`" + `.
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
`
