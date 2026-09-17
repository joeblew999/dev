---
name: dev
description: Build, check, run, release and deploy the commands of a repo on the mise + fnox + hk + packslip stack. Use before running go, npm, wrangler, fly, fnox or goreleaser by hand in such a repo: a mise task names a stage of a command directory and dev does the rest. Tests and releases run the same locally and in GitHub Actions, from the same tasks.
---

# dev

A repo on this stack is a few commands, each its own directory and Go module:
main.go, and beside it a worker.go and wrangler.toml if it deploys to
Cloudflare, a fly.toml if it deploys to Fly, a package.json and .gsx sources if
it has a UI. A repo that is one command keeps it at the root and uses `.`.
Every command has the same stages, and a mise task names one:
`<cmd>:<stage>[:variant]`, so `mise run proxy:deploy` runs
`dev deploy cmd/proxy`. Run stages through their tasks (`mise tasks`
lists them), never by hand, and never call go, npm, wrangler, fly or fnox
directly when a task exists. Anything typed after a task name passes to the
command.

## Verbs

`<cmd> <verb> --help` says what one verb takes, and `<cmd> <verb> <sub> --help`
what one subcommand takes: its flags and what each one
means, read from the flags the verb registers, so they cannot drift from the
code. Here, `mise run help <verb>`.


The directory a verb acts on comes first; flags may follow anywhere, and
everything after a bare `--` goes to the program being run.

