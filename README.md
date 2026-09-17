# dev

The developer tool of a stack: mise for tasks and tools, fnox for secrets, hk
for checks, packslip for releases, Cloudflare Workers or Fly for deploys, Go
for the code, gsx and gsxui for a UI.

A repo on the stack is a few commands, each its own directory. A mise task
names a stage of one, `<cmd>:<stage>`, and `dev <stage> <dir>` reads the
directory and does the rest: a Go main, a package.json, `.gsx` sources, a
`wrangler.toml` or a `fly.toml` each mean something. A new command is new
lines in mise.toml, never new tooling.

## Using it in a repo

```toml
[tools]
"packslip:github.com/joeblew999/dev" = "0.1.0"
```

`mise install` puts `dev` on PATH and links its skill into `.claude/skills/dev`,
so a Claude Code session in that repo knows every verb. The skill is rendered
from the verbs' own usage (`dev skill`), and `dev skill --check` fails when it
drifts.

```toml
[tasks."proxy:build"]
run = "dev build cmd/proxy"

[tasks."proxy:deploy"]
run = "dev deploy cmd/proxy"
```

Run `dev` with no arguments for every verb.

## Working on it

`mise install`, then `mise run test`. The tool is one command at the repo
root, so its own stages take `.`: `mise run release:snapshot` builds and signs
a release locally, `mise run release <version>` publishes one, signed with
the key in fnox (`packslip.pub` is its public half, the `pubkey` consumers
pin). The release workflow runs the same on demand.
