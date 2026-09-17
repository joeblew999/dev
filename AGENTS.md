- Repo: joeblew999/dev

# AGENT instructions

This is the developer tool of the stack. It is used from many repos, so it is

bounded: it knows the stack's conventions and nothing about any one project.

## Speaking tone

You must explain things in easy to understand ways when outputting to a developer.

## Plans

You must go into .plans with date time stamps.

Plans must have steps that are checked off as you do each step, so that any AI can pick up where you left off.

Plans must have a DOD ( Definition of done ).

Plans that are done are moved to the done sub folder.

## Issues

You must raise issues to the developer as you work on a plan or code. 

You can continue working in order to complete the task at hand, but you must provide follow ups in the plan and to the developer, so that we can ensure that issues are not forgotten.

## Mise 

Mise is for orchestrations over the code.  

## Comments

Its important that the comments say why when its neeed. Rational can help use not make the same mistakes. 

## Skills

**The stack's rules live in the dev skill**, [.claude/skills/dev/SKILL.md](.claude/skills/dev/SKILL.md) —
generated from the verbs, shipped to every repo that pins dev, and the one
source of what the tool does and the rules it keeps. Load it before build,
test, release or deploy. This file says only what is about developing the
tool itself.

- **Every workflow is a mise task.** `mise tasks` lists them. `mise run test`
  before committing.
- **Nothing project-specific.** No project, Worker, app or provider names in
  code, tests or messages; a repo reaches the tool through directories,
  `mise.toml` vars and the three tasks it supplies (`check`, `validate`,
  `secrets:list`).
- **The skill is generated.** `dev skill` renders `skills/dev/SKILL.md` and the
  copy in `.claude/skills/dev/` from the verbs' usage; never edit either by
  hand. `mise run check` fails when stale.
- **A package is one thing, named as the tasks name it**, and every verb has
  the one shape in `cli`: `Run(verb, args, stdout, stderr)`.
- **Test for real before saying done.** A cloud path that could not be
  exercised is said so plainly.
- **Commit only files you name.** Never `git add -A`.

## Layout

`cli/` is the public API: the command shape, flags, DIR and `--` passthrough.
Everything else the tool is made of lives under `internal/`.

| Path | Owns |
|---|---|
| `main.go` | the verb table, usage, and `dev skill` |
| `cli/` | the public API: every command's verbs, skill and version |
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
