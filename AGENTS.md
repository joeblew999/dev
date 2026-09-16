- Repo: joeblew999/dev

# Development instructions

This is the developer tool of the stack (see README.md). It is used from many
repos, so it is bounded: it knows the stack's conventions and nothing about
any one project.

- **Every workflow is a mise task.** `mise tasks` lists them. `mise run test`
  before committing.
- **Every error names its fix.**
- **A package is one thing, named as the tasks name it**, and every verb has
  the one shape in `internal/cli`: `Run(verb, args, stdout, stderr)`.
- **Nothing project-specific.** No project, Worker, app or provider names in
  code, tests or messages; a repo reaches the tool through directories,
  `mise.toml` vars and the three tasks it supplies (`check`, `validate`,
  `secrets:list`).
- **Nothing personal in a committed file**, here or in any repo the tool
  touches: cloud credentials come from fnox, provisioned ids never reach git,
  a developer's copy of every app comes from `DEPLOY_SUFFIX` in gitignored
  `mise.local.toml`.
- **The skill is generated.** `dev skill` renders `skills/dev/SKILL.md` from
  the verbs' usage; never edit it by hand. `mise run check` fails when stale.
- **Test for real before saying done.** A cloud path that could not be
  exercised is said so plainly.
- **Commit only files you name.** Never `git add -A`.

## Layout

| Path | Owns |
|---|---|
| `main.go` | the verb table, usage, and `dev skill` |
| `stage/` | build, wasm, check, run, workerd: one command directory, read from what it holds |
| `app/` | url, deploy, logs, smoke, wait: which cloud a directory deploys to, and the dispatch |
| `cloudflare/` | the Workers target: wrangler, the workers.dev subdomain, the throwaway deploy copy |
| `fly/` | the Fly target: flyctl from the repo root, `<app>.fly.dev` |
| `secrets/` | set and push: fnox in, the cloud's CLI out, values never as arguments |
| `session/` | the pinned Claude Code session: sync, check, verify, bump, mcp |
| `release/` | one goreleaser + packslip release by convention: the repo's name, `skills/*` |
| `deps/` | Go module upgrades across a workspace |
| `fnox/` | the one way to a secret; variables so tests can replace them |
| `internal/cli` | the command shape, flags, DIR and `--` passthrough |
| `internal/suffix` | `DEPLOY_SUFFIX` |
