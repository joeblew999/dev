# dev

https://github.com/joeblew999/dev

The developer tool of a stack: mise for tasks and tools, fnox for secrets, hk
for checks, packslip for releases, Cloudflare Workers or Fly for deploys, Go
for the code, gsx and gsxui for a UI.

## Get it

<!-- pin -->

```toml
[tools]
"packslip:github.com/joeblew999/dev" = { version = "<version>", pubkey = "RWQnLlj1BaHu39oBhUFxM3zEscQHqhxihLeTGyDAlavNFAOSTxQEHfoq" }
```

The Releases page has the latest version, and `dev version --pin` prints
this line with the version of the build you already have. This block is
written by `dev skill` from the key the releases are signed with, so it
cannot name a key that no longer signs.

<!-- pin -->

`mise install` puts `dev` on PATH and its skill into `.claude/skills/dev`, so a
Claude Code session in that repo has the manual.

## The manual is the binary

- `dev` — every verb and its usage.
- `mise tasks` — every task of the repo you are in, and what each one does.
- [skills/dev/SKILL.md](skills/dev/SKILL.md) — the same verbs, rendered by
  `dev skill` for Claude Code: **how to use the CLI, readable right here.**
  Never edited by hand; `mise run check` fails when it drifts.

Nothing here restates them.

## Working on it

`mise install`, then `mise run test`. AGENTS.md has the rest.
