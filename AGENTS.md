- Repo: joeblew999/dev

# AGENT instructions

This is the developer tool of the stack. It is used from many repos, so it is
bounded: it knows the stack's conventions and nothing about any one project.

## Modern Go

Refer to https://github.com/JetBrains/go-modern-guidelines and follow its
way, and specifically ensure you install the tooling at
https://github.com/JetBrains/go-modern-guidelines#claude-code — the plugin
`modern-go-guidelines@goland-claude-marketplace`. Its skill builds a small CLI
on first run from a directory outside any repo, so the machine needs a `go`
there: `mise use -g go@1.27.1` once, a global default that every repo's own
pin overrides.


https://www.c-sharpcorner.com/article/go-1-27-generic-methods-where-they-simplify-real-go-apis-and-where-they-dont


This module is `go 1.27.1`, so write 1.22–1.27 Go and not the workarounds that
predate it. An agent's training likely predates 1.27, so when a feature seems
too new to exist, `go doc` it before deciding it does not — the toolchain
mise pins is the authority, not memory.

The guidelines are applied in two halves, and the split is the whole method:

- **`go fix ./...` first.** Every guideline that carries a modernizer is one
  of its analyzers, and it rewrites by syntax tree — the one kind of editing
  Go as text that the refactoring rules below do not warn against. `go fix
  -diff ./...` previews, but read `git diff` after the real run: the preview
  has been seen to miss files the run then touched.
- **The rest by hand, the skill's way:** `list` for the file, `explain` a rule
  before skipping it, and skip only when it would not compile, would change
  behaviour, or plainly does not fit. The record of every rule against this
  tree — applied, no site, or skipped with the rule's own reason — is
  `.plans/done/2026-09-17_2120_modern-go-by-the-guidelines.md`.

Two things the guidelines do not say and this repo learned:

- `httptest.NewTestServer(t, h)` needs `srv.Start()` when the code under test
  brings its own client: its default is an in-memory network that only
  `srv.Client()` can reach.
- The four `encoding/json` imports are v1 on purpose. The `json_v2` rule
  itself says to leave existing code until a migration is asked for, because
  even a compiling import swap can change what goes on the wire.

## Be clear

when you speak back to the dev, be clear !!

## Self reflection

MAKE sure that the code and usage line up well, so that devs and agents will get a proper cli and skills experience !!


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
- **The prose is markdown beside the code it describes**, always: a package's
  verbs in its own `usage.md`, the manual around them in `skill.md` beside `main.go`, a library's manual in that library's directory.
  All compiled in by `go:embed`. A file added there goes in `mise.toml`'s
  build `sources`, or editing it leaves the binary stale while mise reports it
  fresh.
- **Work goes through the cloud's CLI; questions it cannot answer go to the
  API.** `flyctl` and `wrangler` build images, upload Workers, push secrets
  and hold the credential while they do it. The registry pins them, so their
  versions are controlled there, and replacing that with a Go client would
  control the same thing twice.

  That argument only covers what a CLI does. It does not cover zones, DNS,
  SSL settings, listing an account's Workers, querying stored telemetry or
  reading the workers.dev subdomain — `wrangler` does none of those, and dev
  already calls the API directly for all six. An earlier version of this rule
  said "never a Go client" and gave the registry as the reason, which was
  wrong: for those six there is no binary to duplicate, and hand-rolled HTTP
  is a Go client too, only one nobody generated.

  For that half, the thing that decides is what kind of dependency it is. A
  CLI is a binary: the registry fetches it, it runs as its own process, and it
  costs `go.mod` nothing. `cloudflare-go` and `fly-go` are libraries — no
  `cmd/`, no executable, nothing to install — so they are compiled into dev
  and they do add modules. That is not a size argument and does not turn on
  how many modules: it is the difference between a tool dev runs and code dev
  becomes.

  Measured, because this was argued four times on reasoning and settled in
  minutes on numbers. A throwaway module importing cloudflare-go/v7, against
  the same program written with net/http:

  | | dev today | with cloudflare-go |
  |---|---|---|
  | warm rebuild | 1.1s | 2.1s |
  | cold build | 3.7s | 18.4s |
  | binary | 11.3 MB | ~28 MB |
  | modules | 4 | 9 |

  The warm rebuild is the one that decides it. Everything about working here
  rests on a build of about a second — it is why `mise run check` is worth
  running after every edit — and doubling the inner loop taxes every edit for
  as long as the dependency lives. cloudflare-go's own go.mod is tidy, two
  direct dependencies; the weight is the generated SDK itself, which covers
  every Cloudflare product and cannot be linked away.

  fly-go is worse and not close: nine direct requires and twenty-odd indirect,
  including the whole Prometheus client stack, OpenTelemetry and protobuf — to
  replace a flyctl that already does everything and costs go.mod nothing.

  And there is no third door. A binary would have settled everything — the
  registry fetches binaries and they cost go.mod nothing — so it was worth
  looking properly rather than assuming. Three checks, all agreeing:
  Cloudflare dropped flarectl in January 2025 (present through v0.115.0, gone
  from v4.0.0); mise's registry knows 1050 tools and has only cloudflared,
  wrangler, workerd and cfssl; and GitHub has no maintained general-purpose
  Cloudflare CLI in any language.

  Which means dev is that binary. It already ships through packslip and other
  repos on the stack install it from the registry, so when one of them wants a
  zone checked it runs `dev fronting`, exactly as it runs flyctl — a binary,
  costing their go.mod nothing. The 256 lines are not a workaround for a
  missing CLI; they are the CLI, in the tool that needed one. A separate `cf`
  binary would be the same code plus its own release machinery. Cloudflare did ship a general-purpose CLI — `flarectl`, inside
  cloudflare-go at `cmd/flarectl` — and dropped it in January 2025 when the
  package became a generated SDK: present through v0.115.0, gone from v4.0.0
  on. Nothing replaced it. `wrangler` is Workers only, `cloudflared` is
  tunnels only, and no maintained general-purpose Cloudflare CLI exists.

  So the API half stays hand-rolled, not because a library was weighed and
  rejected, but because the only alternative to hand-rolled HTTP is a library.
  It is 256 lines and one generic sender for six endpoints, and the cost is
  naming fields correctly — a real cost, paid twice in one day. If that ever
  stops being the right call the argument has to be that the hand-rolled code
  is unsafe, not that an SDK is convenient.

  The cost of a CLI is parsing output, so prefer `--json` and decide from what
  parsed. Never decide from a tool's prose: this tree has been wrong twice
  about what `flyctl` says and which stream it says it on, and once about an
  empty list meaning a thing was absent.

## Refactoring without breaking it

Three times in one day a refactor here was done by editing Go as text — a
blanket string replace, a regex over a line range, an index-and-slice rewrite
— and each time it corrupted a file that had been fine. What saved it every
time was `git checkout --`, which only worked because the tree was committed
and green first.

- **Commit green before starting.** The tree you can return to is the whole
  safety net.
- **One file at a time, then `go build ./...`.** It takes about a second. A
  batch of edits with one build at the end says something broke and not which
  edit did it.
- **`mise run check`, not `mise run build`.** check depends on build, so it
  compiles, writes the manual *and* runs every test, for under two seconds
  more. build verifies nothing — it was written as the fast inner loop, and
  reaching for it all day is how a broken test survives until the commit
  refuses it. Nothing here is slow enough to be worth that.
- **Edit by exact match, not by pattern.** `stdout` → `c.Stdout` across a file
  hits function parameters too. If a change cannot be written as an exact
  replacement of text you have just read, it is too big to do in one step.
- **A commit runs it all.** hk's pre-commit is gofmt, vet, tidy, staticcheck,
  whitespace, secrets and large files, and `mise run lint` runs that same list
  from `hk.pkl` — one place, so the two can never disagree. CI runs
  `mise run test`, which is that plus the tests and a signed snapshot release.
- **`mise run lint` says what `go vet` will not** — a function nobody calls, a
  variable a refactor left behind, a deprecated call. Every one of those lived
  in this tree until staticcheck was added to the gate.
- **`mise run dead` is a report, not a gate.** Dead code is normal mid-refactor;
  the point is to see what a change left behind, not to fail on it.


## Layout

| Path | Owns |
|---|---|
| `main.go` | the verb table, its order, and the prose around it |
| `cli/` | the public API: verbs, the manual, the markdown it is written in |
| `internal/stage/` | build, wasm, check, run, workerd: one command directory, read from what it holds |
| `internal/app/` | url, deploy, list, logs, smoke, wait, domains, fronting: which cloud a directory deploys to, the dispatch, waiting for an address that belongs to neither, and what stands in front |
| `internal/cloudflare/` | the Workers target through wrangler, and the API for what wrangler cannot answer: zones, DNS, SSL mode, telemetry, the workers.dev subdomain |
| `internal/fly/` | the Fly target: flyctl from the repo root, `<app>.fly.dev` |
| `internal/secrets/` | set and push: fnox in, the cloud's CLI out, values never as arguments |
| `internal/session/` | the pinned Claude Code session: sync, check, verify, bump, mcp |
| `internal/release/` | one goreleaser + packslip release by convention: the repo's name, `skills/*` |
| `internal/deps/` | Go module upgrades across a workspace |
| `internal/fnox/` | the one way to a secret; variables so tests can replace them |
| `internal/gitignore/` | `.gitignore` lines the tool owns |
| `internal/gitrepo/` | which repository a directory is in, from its git remote |
| `internal/suffix/` | `DEPLOY_SUFFIX` |
