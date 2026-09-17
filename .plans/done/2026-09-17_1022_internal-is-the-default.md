# `internal/` is the default: only `cli` is API

**File:** `dev/.plans/2026-09-17_1022_internal-is-the-default.md` — refer to this plan by that name.

**Status:** done 2026-09-17 — moved, green, committed (see log) ·
**Created:** 2026-09-17 10:22 · **Finished:** 2026-09-17
**Decision taken:** `cli` is the whole public surface; `fnox` moved to
`internal/fnox` with the rest, per the recommendation.
**Affects:** this repo only — ten directories move under `internal/`, ~20 Go
files get a rewritten import line, and the Layout table in `AGENTS.md`. No
behaviour, no task, no generated skill, no release artifact, no other repo.
**Decisions needed before starting:** one — is `cli` the whole public surface,
or does `fnox` stay out with it? Recommendation below.

## Decision and evidence

The question was whether the Go here follows the conventions. Most of it does,
and the parts that do are worth naming so they are not "tidied" away:

| Already right | Why it stays |
|---|---|
| `main.go` at the repo root | This repo is one command, and the stack's own rule is that a one-command repo keeps it at the root and takes `.`. A `cmd/dev/` would make `dev` the one repo that breaks the convention `dev init` writes into every other one, and `dev release .` generates `main: .` against that root. |
| no `pkg/` | `pkg/` is a popular repo layout, not a Go one. Nothing in the toolchain knows it. |
| one thing per package, named as the tasks name it | Already the rule in `AGENTS.md`, already kept. |
| a package comment on every package | All fourteen have one. Most repos do not. |
| `proc_unix.go` / `proc_other.go`, `exec_unix.go` / `exec_other.go` | Build-tag pairs are how this is done. |
| tests beside the code, in the package | Right for a tool whose tests reach internals (`release/key_test.go` replaces `fnox.Get`). |

**One thing is actually wrong, and it is the module's public surface.** Ten
directories sit at the module root — `app`, `cloudflare`, `deps`, `fly`,
`fnox`, `release`, `scaffold`, `secrets`, `session`, `stage` — so
`github.com/joeblew999/dev/cloudflare` is an importable package that anyone
may depend on and `go doc` presents as supported. None of them is meant for
that. They are the verbs' guts: `stage.Run`, `app.Run`, `release.Run` exist
because `main.go` puts them in a verb table.

Exactly one package *is* meant to be imported: `cli`, which was moved out of
`internal/` in the working tree on purpose — "Every command gets skill and
version without writing them" is a promise to other repos' commands, and it is
the one promise this module makes.

The direction matters, and it is why now is the moment:

- **`internal/` → public is additive.** Promoting a package later breaks nobody.
- **public → `internal/` breaks every importer.** Doing it once there are any
  is a major-version event.

So the cheap, reversible choice is to start everything internal and promote on
demand. Today there are no importers to break (proven below); in a release or
two there may be.

**`fnox` is the one honest call.** "The one way to a secret" sounds like
something another repo would want. Against: nothing imports it, and its API is
`var Get = func(...)` — package-level variables so tests can swap them. That is
a seam, not a contract, and publishing it promises a shape that exists for the
tests. Recommendation: `internal/fnox` now, promote free of charge the first
time a repo actually needs it.

## What was proven on 2026-09-17

- **Nothing outside this repo imports any of it.** Every `*.go` and `go.mod`
  under `~/workspace/go/src/github.com/joeblew999` naming `joeblew999/dev` is a
  file inside `dev` itself. The tool ships as a signed binary that `mise`
  installs from a packslip release — repos pin the artifact, not the module.
  (Limit: this proves the machine, not GitHub. The module is days old and the
  README documents only the packslip pin.)
- **No internal type would leak into `cli`'s API.** `cli` imports nothing from
  this module — the verb table holds *function values* (`stage.Run` satisfying
  `cli.Runner`), never a type from a moving package. So `cli` stays compilable
  and importable with every other package behind `internal/`.
- **`//go:embed all:files` moves with its file.** The embed path is relative to
  `scaffold.go`, so `scaffold/files/` travels with the package untouched.
- **The release does not care.** `goreleaser` is generated with `dir: .` and
  `main: .`; the root keeps `main.go` and `main_test.go`.

## The move

```
app/ cloudflare/ deps/ fly/ fnox/ release/ scaffold/ secrets/ session/ stage/
                                        →  internal/<same name>/
```

Staying put: `main.go`, `main_test.go`, `cli/`, `internal/gitignore/`,
`internal/gitrepo/`, `internal/suffix/`, `skills/`.

The root then reads: a `main.go` that is the verb table, a `cli/` that is the
API, an `internal/` that is the tool, and `skills/` that is what ships beside
the binary.

## Steps

- [x] 1. `git mv` each of the ten directories under `internal/`, one command each, so
   history follows the files. Done 2026-09-17.
- [x] 2. Rewrite the import lines, matching on the closing quote
   (`joeblew999/dev/<pkg>"` → `joeblew999/dev/internal/<pkg>"`). Done 2026-09-17:
   15 files; template literal `packslip:github.com/joeblew999/dev` and
   `strings.Contains(tool, "joeblew999/dev")` untouched, verified.
- [x] 3. `gofmt -w .`. Done 2026-09-17: clean.
- [x] 4. `mise run test`. Done 2026-09-17 as `go vet ./...` (clean) +
   `go test ./...` (all green) + `.bin/dev skill --check` (three copies up
   to date). One fix: `internal/stage/stage_test.go` read `go.sum` via
   `filepath.Abs("..")` — now `"../.."`. Full `mise run test` (snapshot
   release) not run; needs fnox/network.
- [x] 5. Update the Layout table in `AGENTS.md`. Done 2026-09-17: all ten
   moved rows renamed, `internal/scaffold`, `internal/gitignore`,
   `internal/gitrepo` added, `cli/` row rewritten as the public API.
- [x] 6. One commit, naming the files. Done 2026-09-17 (see log).

## DOD

- [x] Root reads `main.go`, `cli/`, `internal/`, `skills/` — no other
  importable package.
- [x] `go vet ./...` clean, `go test ./...` green, `gofmt` clean,
  `dev skill --check` green.
- [x] `AGENTS.md` Layout table matches the tree.

## Also missing, decide separately

**There is no `LICENSE`.** A public module that other repos pin, on a public
repo, with no license is by default all-rights-reserved. This is not layout and
does not belong in the same commit, but it is the other thing a Go repo of this
kind is expected to have.

**`go.mod` says `go 1.27.1`, `mise.toml` says `go = "1.27"`.** Legal, and the
patch level in the `go` directive pins a toolchain rather than a language
version. Worth making one of them the source of the other, or leaving a line
saying why they differ. Not part of this move.

## Risk

Low. A rename plus an import rewrite, proven by a test task that already runs
`vet`, the tests and a real signed snapshot build. The one way to get it wrong
is a sed that eats a template string, which step 2 is written to prevent and
step 4 would catch.
