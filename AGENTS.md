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

  Two near-misses, both worth recording so they are not re-found and mistaken
  for answers. cloudflare-cli4 is Cloudflare's own generic CLI over the v4
  API and is dead the same way flarectl is — last release May 2024, and the
  library under it archived. dnscontrol is alive and excellent (Go, a binary,
  pushed this week) and is the wrong model: it is declarative whole-zone sync,
  so pointing it at a zone to add one record invites it to delete everything
  not in the config, and it does not touch SSL mode, which is not DNS.

  What is well served by binaries is tunnels: cloudflared is in the registry
  as aqua:cloudflare/cloudflared, and wrangler has a tunnel command. When
  fronting an app through a tunnel arrives, it is a pinned binary and neither
  a library nor hand-rolled HTTP.

  Which vendor keeps what alive is the thing to look up, and they chose
  opposite answers. Cloudflare maintains a Terraform provider — official, a
  thousand stars, pushed this month — and killed both of their CLIs. Fly
  maintains flyctl and archived their provider in November 2023; what exists
  now is a single-maintainer revival with no adoption, and putting
  infrastructure on it to replace an official CLI that already does
  everything would be a poor trade.

  So dev uses tofu for Cloudflare and flyctl for Fly, and that is not an
  inconsistency to tidy up later. It is following the maintenance.

  For writing, there is a binary after all, and it is the obvious one once
  somebody says it: opentofu with cloudflare's own provider. Both are fetched
  by the registry — aqua:opentofu/opentofu, and the provider by tofu itself —
  so neither costs go.mod anything, and the token reaches them through fnox
  exec like every other tool here.

  It is the right shape for writes in a way hand-rolled POSTs are not. `tofu
  plan` says exactly what would change before anything does; a plan against a
  real zone here read "2 to add, 0 to change, 0 to destroy", which is the
  guarantee that matters — Terraform manages the resources declared and
  nothing else, so it is not the whole-zone sync that makes dnscontrol
  dangerous here. State records what dev created, so removing it is exact.

  It also surfaces what a hand-rolled PATCH would not: cloudflare_zone_setting
  cannot be destroyed by Terraform, so changing a zone's SSL mode is one-way
  through that path. Knowing that before running it is the argument for plan.

  Reading stays dev's own, because a read wants no state file, no plan and no
  provider — it wants an answer. Which means dev is that binary for reads. It already ships through packslip and other
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

## DRY and generics, without being asked

This is the standing instruction, not a preference to be reminded of. After
any change, look for what it duplicated and collapse it — and do the same for
what was already there beside it.

- **A fact belongs in one place.** Two spellings of one fact drift the day
  either is edited: a tool's mise pin written as both a spec and a TOML line,
  a verb's subcommands listed in Subs and again in a switch, a manual's verbs
  rendered twice. Derive the second from the first or delete it.
- **Three of a shape is a generic.** Not two — two is a coincidence. Four
  reports in this tree built a step by hand: start a clock, call the thing,
  stop the clock, fill a Step, record it. The fifth line varied only by being
  forgotten, which is what duplication actually costs.
- **Generic methods exist.** Go 1.27 allows a type parameter on a method, so
  `res.JSON[T]("what")` rather than a package function taking the receiver.
  When a feature seems too new, `go doc` it rather than trusting memory.
- **Generics for the shape, functions for the work.** `Map`, `Filter`,
  `Collect`, `Sorted`, `Parallel`, `Gather` describe shapes. What runs inside
  them is ordinary code.
- **The pair keeps recurring: reconcile and establish.** `session sync` and
  `session remove`, `front` and `unfront`, `tools --add` and `--fresh`. One
  moves toward a declared state, the other makes it. When writing the first,
  ask what the second is.
- **A registry is one declaration everything else reads.** Adding a cloud, a
  checker, a writer or a tool is adding one entry — and a test holds that by
  failing when something reads around it. `Target` chose by naming both config
  files itself, so a third cloud was an entry plus an edit nobody would think
  to make.

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
- **`mise run audit` is golangci-lint, and `check` depends on it.** Seventeen
  linters that nothing else here runs, with `default: none` so staticcheck is
  not declared in two files that could disagree. It is a gate and it sits at
  zero: four of them found something and it was fixed, the other thirteen
  found nothing and are there for the day something appears. Warm it takes
  under a second, which is why it belongs in the inner loop and not in a task
  somebody remembers.

  `dupl` is deliberately not among them. golangci-lint runs analyzers per
  package, so its dupl cannot see across package boundaries: measured on this
  tree it found no cross-file clones at any threshold, where standalone dupl
  finds the real one — `internal/cloudflare/deploy.go` against
  `internal/fly/fly.go`, the same step written twice in the two cloud targets.

- **`mise run lint` says what `go vet` will not** — a function nobody calls, a
  variable a refactor left behind, a deprecated call. Every one of those lived
  in this tree until staticcheck was added to the gate.
- **The tests reach nothing.** `check` runs them behind a dead proxy —
  `{{ vars.sealed }}` in mise.toml — with localhost excluded so `httptest`
  still works. Every test here is fixtures, stubs or a local server, and that
  was true by intention and checkable by nobody until the gate started
  proving it on every run. A test that talks to a cloud passes on its own and
  fails under `check`, which is the right way round: the one that reaches a
  real account is the one that should be hard to land.

  What is pointed at a real thing is the mise tasks, not the tests:
  `site:*:check` and `site:health` take their origin from `[vars]`, so they
  can be aimed anywhere, and the tests cannot be aimed at all. A `Call` built
  by hand reads no environment, so running the suite from a task that sets
  `DEV_URL` does not quietly test a different site than the fixtures
  describe.

- **`mise run dead` is a report, not a gate.** Dead code is normal mid-refactor;
  the point is to see what a change left behind, not to fail on it.
- **`mise run smells` is all four of them**, and they are all reports: `dead`,
  `dup`, `unread` (a parameter the body never touches — a signature promising
  what the code does not do) and `repeated` (one fact spelled out in three
  files). None of them fails a build and none is in `hk.pkl`, so read them
  after a refactor rather than running them to green. About half of what the
  last two say is fine as written; reading past that half is the price of the
  other one.


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
