# dev

The developer tool of a stack: mise for tasks and tools, fnox for secrets, hk
for checks, packslip for releases, Cloudflare Workers or Fly for deploys, Go
for the code, gsx and gsxui for a UI.

## Get it

```toml
[tools]
"packslip:github.com/joeblew999/dev" = "<version>"   # the Releases page has the latest
```

`mise install` puts `dev` on PATH and its skill into `.claude/skills/dev`, so a
Claude Code session in that repo has the manual.

## The manual is the binary

- `dev` — every verb and its usage.
- `mise tasks` — every task of the repo you are in, and what each one does.
- [skills/dev/SKILL.md](skills/dev/SKILL.md) — the same verbs, rendered by
  `dev skill` for Claude Code: **how to use the CLI, readable right here.**
  Never edited by hand; `mise run check` fails when it drifts.

Nothing here restates them. `dev init` writes the stack into a new repo;
`github.com/joeblew999/hello-stack` is exactly that and nothing else, the
reference hello world.

## Working on it

`mise install`, then `mise run test`. AGENTS.md has the rest.
