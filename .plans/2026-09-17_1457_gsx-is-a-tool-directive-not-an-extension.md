# gsx is a tool directive, not a file extension

**Status:** not started · **Created:** 2026-09-17
**Affects:** `internal/stage/stage.go` (`Inspect`, `Build`, `Check`),
`internal/stage/usage.md`, `internal/stage/stage_test.go`.

## What the tool believes today

`Inspect` decides a command directory needs gsx from the files it holds —
`internal/stage/stage.go:86-87`:

```go
gsx, _ := filepath.Glob(filepath.Join(path, "*.gsx"))
d.GSX = len(gsx) > 0 || exists(filepath.Join(path, "gsx.toml"))
```

When that is true, `Build` runs `go tool gsx generate -q` and `Check` runs
`go tool gsx fmt -l .` in the directory.

## Why that is not safe

`.gsx` is not gsx's extension. It is an extension two projects chose
independently:

| | `github.com/gsxhq/gsx` | `github.com/grindlemire/go-tui` |
|---|---|---|
| authoring file | `.gsx` | `.gsx` |
| generated file | `.x.go` | `*_gsx.go` |
| generate | `go tool gsx generate` | `tui generate` |
| format | `gsx fmt` | `tui fmt` |
| renders to | HTML | a terminal character grid |

Nobody owns the extension, so reading a toolchain out of it is a guess. Two
ways the guess goes wrong:

1. **A repo uses go-tui and not gsx.** `dev build` runs `go tool gsx
   generate` in a module that never declared gsx, and go answers "no
   required module provides tool …". The developer is told about a tool they
   never asked for, and the message does not name the fix. The stack's rule
   is that every error names its fix.
2. **A repo uses both** — a Worker with a gsx UI, and a TUI command beside
   it. Now gsx *is* a tool dependency, so nothing fails early: gsx is handed
   go-tui's `.gsx` files and reports parse errors on a file it was never
   meant to read. `dev check` does the same through `gsx fmt -l`, calling
   those files unformatted. This is the bad one, because the error blames
   the file rather than the inference.

Neither is happening today: no repo on the stack uses go-tui. This is a
latent wrong inference, not a live bug.

## What is actually true

`go tool gsx` resolves against one thing — a `tool` directive in the
enclosing module's `go.mod`. Every gsx-using module on the stack has it:

    tool github.com/gsxhq/gsx/cmd/gsx

verified in `auth-proxy/cmd/gui`, `go-htmx4`, `gsxui`, `gsxui/site/hl/gen`,
`gsx` and `gsx/playground/server`.

So the tool directive is not a second copy of the fact — it *is* the fact,
the same one `go tool gsx` reads. `.gsx` files say what the source language
looks like; the directive says which compiler is pinned. Only the second one
answers the question `Inspect` is asking.

## The wrinkle: which module

A command directory is not always its own module. `auth-proxy/cmd/gui` has
its own `go.mod`; `go-htmx4` keeps `go.mod` at the root with `.gsx` sources
under `ui/`. So the check must walk up from DIR to the nearest `go.mod`, the
way the go command itself resolves a module — not just stat `<DIR>/go.mod`.

Two ways to read it, and the choice is worth making deliberately:

- **Parse `go.mod`** (walk up, look for the directive). Stays pure
  filesystem, so `Inspect` remains exec-free and `stage_test.go` keeps
  writing temp directories to drive it.
- **Ask the toolchain** (`go tool` in DIR lists the available tools). More
  honest — it is the same resolution go itself does — but it puts an exec
  inside `Inspect`, which today is pure, and every test that builds a fake
  directory would need a module that really resolves.

Leaning to the first for that reason, with a comment saying why.

## DOD

- [ ] `Dir.GSX` is true only when the enclosing module pins
      `github.com/gsxhq/gsx/cmd/gsx` as a tool.
- [ ] A directory of `.gsx` files with no gsx tool directive is not treated
      as a gsx directory, and nothing runs gsx in it.
- [ ] `stage_test.go` covers both: `.gsx` with the directive (gsx runs) and
      `.gsx` without it (gsx does not).
- [ ] `internal/stage/usage.md` no longer says or implies that `.gsx`
      sources alone select gsx; it says what actually selects it.
- [ ] The `Dir.GSX` field comment says the directive, not the glob.
- [ ] `mise run test` green.

## Work

- [ ] 1. A helper that finds the enclosing `go.mod` from a directory and
      reports whether it declares the gsx tool. Its own test.
- [ ] 2. `Inspect` uses it for `d.GSX`. Decide what `gsx.toml` still means —
      probably nothing on its own, since a `gsx.toml` beside no pinned gsx
      is a repo that cannot run gsx anyway.
- [ ] 3. Tests for both directions.
- [ ] 4. `usage.md` and the field comment in the same edit.
- [ ] 5. `mise run test`; record results here.

## Issues raised

- Nothing is filed with gsxhq/gsx or grindlemire/go-tui. Neither has a bug:
  each generator is correct about its own files. The wrong inference is
  `dev`'s, so the fix is `dev`'s.
