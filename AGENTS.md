- Repo: joeblew999/dev

# AGENT instructions

This is the developer tool of the stack. It is used from many repos, so it is
bounded: it knows the stack's conventions and nothing about any one project.

## The stack's rules live in the dev skill

[.claude/skills/dev/SKILL.md](.claude/skills/dev/SKILL.md) is generated from
the verbs and ships to every repo that pins dev. It holds how to work on a
repo on this stack — tone, plans, issues, mise tasks, comments, testing for
real, committing only named files, and keeping a verb's usage true to its
code. Load it before build, test, release or deploy, and put a rule there
rather than here when it holds for any repo, so that fixing it once fixes it
everywhere.

Two of its rules are worth naming here because they are the ones most easily
skipped, and this file is read every session while the skill is read on
demand:

- **Keep the code and its usage right.** Change a flag, an argument or a
  behaviour and change that verb's `usage.md` in the same edit. Tests hold the
  manual to the verbs and the markdown to its shape; nothing can check that
  the words are true. Read the usage against the code before calling a verb
  done.
- **Explain things in easy to understand ways** — about the code, and about
  what you are doing and asking. A question a developer cannot parse is a
  question you have not finished writing.

This file says only what is about developing the tool itself.

## Developing the tool

- **Nothing project-specific.** No project, Worker, app or provider names in
  code, tests or messages; a repo reaches the tool through directories,
  `mise.toml` vars and the three tasks it supplies (`check`, `validate`,
  `secrets:list`).
- **`cli/` is the public API** — the command shape, flags, DIR and `--`
  passthrough — and every other repo on the stack builds its commands on it,
  so a change there reaches them all. Everything else lives under `internal/`.
- **The prose is markdown beside the code.** Each package's verbs are
  documented in its own `usage.md`, the manual's surrounding prose in
  `skill/head.md` and `skill/tail.md`, all compiled in by `go:embed`. A file
  added there goes in `mise.toml`'s build `sources`, or editing it leaves the
  binary stale while mise reports it fresh.

## Layout

| Path | Owns |
|---|---|
| `main.go` | the verb table, its order, and the prose around it |
| `cli/` | the public API: verbs, the manual, the markdown it is written in |
| `internal/stage/` | build, wasm, check, run, workerd: one command directory, read from what it holds |
| `internal/app/` | url, deploy, logs, smoke, wait: which cloud a directory deploys to, and the dispatch |
| `internal/cloudflare/` | the Workers target: wrangler, the workers.dev subdomain, the throwaway deploy copy |
| `internal/fly/` | the Fly target: flyctl from the repo root, `<app>.fly.dev` |
| `internal/secrets/` | set and push: fnox in, the cloud's CLI out, values never as arguments |
| `internal/session/` | the pinned Claude Code session: sync, check, verify, bump, mcp |
| `internal/release/` | one goreleaser + packslip release by convention: the repo's name, `skills/*` |
| `internal/deps/` | Go module upgrades across a workspace |
| `internal/fnox/` | the one way to a secret; variables so tests can replace them |
| `internal/scaffold/` | `dev init`: the stack's files and the first command, from embedded templates |
| `internal/gitignore/` | `.gitignore` lines the tool owns |
| `internal/gitrepo/` | which repository a directory is in, from its git remote |
| `internal/suffix/` | `DEPLOY_SUFFIX` |
