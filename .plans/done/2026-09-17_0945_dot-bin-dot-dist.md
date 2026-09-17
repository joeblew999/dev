# Build output goes under a dot: `bin/` becomes `.bin/`, `dist/` becomes `.dist/`

**File:** `dev/.plans/2026-09-17_0945_dot-bin-dot-dist.md` — refer to this plan by that name.

**Status:** ready to start, no open decisions — **every risky assumption
verified for real on 2026-09-17 09:55**, see "What was proven" · **Created:** 2026-09-17 09:45
**Affects:** this repo (`stage`, `release`, `deps`, a new `internal/gitignore`,
`mise.toml`, `.gitignore`, the scaffold templates and the generated skill) and every repo that pins `dev`
— their `mise.toml` `outputs` and their `.gitignore`.
**Decisions needed before starting:** none. Decision 2 was settled by the
Product Owner on 2026-09-17: `dev` owns the convention and asserts `.dist`
rather than parsing for it.

## Decision and evidence

Two directories hold build output at a repo root, and both sort in among the
source: `bin/` (what `dev build` writes and `dev run` executes) and `dist/`
(what goreleaser writes and packslip signs). Both become dot-directories, so a
listing of a repo root shows what a developer wrote and nothing a tool made.

They are **not** symmetric, and that is the whole shape of the work:

| | who decides the path today | how many places name it |
|---|---|---|
| `bin/` | `dev` itself — `stage` builds to it and runs from it | 3 in `stage/stage.go`, plus `mise.toml`, `.gitignore`, the two scaffold templates, one CI comment |
| `dist/` | **goreleaser**, from the `dist:` key of its config | 6 in `release/release.go`, 2 in `release/release_test.go`, `.gitignore`, `.gitignore.tmpl` |

`bin/` is a rename. `dist/` is a rename *plus* a missing abstraction: today
`release.go` hardcodes the literal `dist` in six places
(`release/release.go:284`, `:286`, `:300`, `:359-360`, `:444-445`) while the
directory is really goreleaser's to choose. That is already a latent bug, and it is **not hypothetical — it was
reproduced on 2026-09-17**: with a `.goreleaser.yml` in this repo saying
`dist: .dist`, goreleaser built and archived all six targets successfully and
`dev release . --snapshot` then failed with

    error: no dist/*.tar.gz to sign; goreleaser built nothing

The message is not merely unlucky, it is *false* — goreleaser had just built
six archives. This change is what forces the bug into the open, because the
generated config will set `dist: .dist`.

`release.go` already has the precedent for reading a repo's config rather than
assuming: `binaries()` (`release/release.go:208-215`) parses every `binary:`
line out of a repo-supplied config for exactly this reason. `distDir()` is the
same move for one more key.

### Decision 1 — no compatibility fallback

`dev build` writes `.bin/<name>`; `dev run` executes `.bin/<name>` and nothing
else. No "try `.bin/`, fall back to `bin/`". One shape, and when the binary is
missing the error names the fix — `dev build DIR` — the way every other error
here does. A consuming repo that has not migrated gets a clear failure on the
first `dev run`, not a silent run of a stale binary from the old path.

### Decision 2 — SETTLED 2026-09-17: `dev` owns the convention

The Product Owner's call: *"our stack is simple, we can assume that `dev`
controls the generation of the `.goreleaser.yml`... because of the way `dev`
works, we and other repos can gen it where any `main.go` is — that's the whole
point of the design."*

That is exactly what the code does, and it is worth naming, because it is what
makes this decision safe rather than merely convenient:
`goreleaserConfig(name, dir, pubkey)` emits `dir: <DIR>` with `main: .`, so
`dev release cmd/foo` synthesises a config for whatever directory holds the
`main.go`, writes it to a temp file, and commits nothing to the repo. The
scaffold ships that shape by default — `run = "dev release cmd/__NAME__
--snapshot"` (`scaffold/files/mise.toml.tmpl:121`). A repo on this stack
normally has **no** `.goreleaser.yml` at all.

So `dist` is `dev`'s to state, everywhere it matters. `.dist` is not parsed,
it is **asserted**: `r.dist` is always `.dist`, there is no `distDir()` and no
regexp.

That leaves one case to handle rather than ignore: a repo that *does* commit
its own `.goreleaser.yml`. Two do today — gsx (`gsx`, `gsx-typebundle`) and
gsxui (`gsxui`, `stylegen`), both pinning `dev`, which is why the read branch
at `release/release.go:184-190` exists at all and why `release_test.go:54`
covers multi-binary manifests. Neither sets a `dist:` key, so both default to
`dist/` and would break the moment `dev` looks in `.dist/`.

`dev` therefore checks, and the error names its fix, as every error here does:

    .goreleaser.yml does not set `dist: .dist`; add that line (dev writes
    build output under a dot, so .gitignore and mise outputs agree)

One check, no parsing, and the convention is `dev`'s rather than each repo's.
The cost is one line added to two files, at the same moment those repos are
already being edited for `.bin` — see step 7.

**Not in this plan:** making `dev`'s generated config handle multiple binaries
so gsx and gsxui could drop their configs entirely. That is the real end state
of "dev controls generation", but it is a separate change with its own risk,
and bundling it would make this diff about two things.

## Steps

Each step is a commit, and `mise run check` is green at the end of each.

1. **`stage/` builds and runs under `.bin/`.** Add one exported constant —
   `BinDir = ".bin"` — and use it at `stage/stage.go:147` (`go build -o`),
   `:218` (`fnox exec --`) and `:255` (the smoke probe). Update the three
   comments that say `bin/` (`:28`, `:29`, `:118`) and, importantly, the two
   `Usage` lines (`:263`, `:266`) — those are what the skill is rendered from.
   Exporting the constant matters: the scaffold's `mise.toml.tmpl` and this
   repo's own `mise.toml` name the same path in `outputs`, and a reader should
   be able to find the one place that decides it.

2. **`release/` moves to `.dist`.** Add `dist: .dist` to the generated config
   (`goreleaserConfig`, `release/release.go:223`) and replace the six `dist`
   literals with the one constant `DistDir = ".dist"`. In the read branch
   (`release/release.go:184-190`), assert the repo's config sets
   `dist: .dist` and fail with the sentence in Decision 2 when it does not —
   a plain `strings.Contains`-grade check on the bytes already read for
   `binaries()`, no new parsing. Fix the "no %s/*.tar.gz to sign" error to
   name the real directory; it currently claims goreleaser "built nothing"
   when goreleaser built six archives. `release/release_test.go:32,36` move
   to `.dist/`.

3. **`deps/deps.go:47`** gains `.bin` and `.dist` to `skipped`. Keep `dist`,
   `bin` and `build` there — `node_modules`-style output and unmigrated repos
   still exist, and the map costs nothing.

4. **This repo's own files.** `.gitignore`: `bin` → `.bin`, `/dist/` →
   `/.dist/` (keeping each line's existing anchoring). `mise.toml`: the
   `build` task's `description`, `outputs` and `run` (`:34-37`), and the four
   `bin/dev` invocations in `check`, `skill` and the two release tasks
   (`:49-50`, `:60`, `:65`, `:70`). `.github/workflows/test.yml:25` — the
   comment that says "postinstall builds bin/dev".

5a. **`dev` puts the lines in `.gitignore` itself** — `internal/gitignore`,
   `Ensure(root, entries...)`. `stage.Build` ensures `.bin` before it writes a
   binary; `newRelease` ensures `.dist` before goreleaser writes archives. It
   compares patterns with anchoring and trailing slashes stripped, so `.bin`,
   `/.bin/` and `.bin/` are one answer and it never appends a line a repo
   already has in another shape; with nothing to add it does not touch the
   file, so a build is not a write. **This is what takes `.gitignore` out of
   the migration entirely**: an unmigrated repo heals the first time anyone
   builds in it, rather than waiting for someone to remember.

5. **The scaffold templates**, so a new repo is born with the convention:
   `scaffold/files/.gitignore.tmpl:1-2` and
   `scaffold/files/mise.toml.tmpl:128,130`.

6. **Regenerate the skill — now two copies.** `mise run skill` rewrites
   `skills/dev/SKILL.md` *and* `.claude/skills/dev/SKILL.md` from step 1's
   `Usage` strings — never by hand. Both are checked, by the two
   `bin/dev skill --check` lines in `mise.toml`'s `check` task, so a stale
   copy of either goes red. (This second copy and the `--out=` flag behind it
   arrived in uncommitted in-flight work found in the tree on 2026-09-17; the
   plan is written against it, and step 4's `mise.toml` edits must preserve
   both lines rather than reinstating the single one.)

7. **Migration note for the repos that pin `dev`** — **deferred by the Product
   Owner on 2026-09-17: "we will fix the other repos later."** Steps 1–6 land
   and release on their own; this step is the written record of what those
   repos will need when someone gets to them, and it does not gate the work.
   Nothing here is code. Three
   repos pin it — auth-proxy, gsx and gsxui (checked 2026-09-17) — and they do
   **not** all need the same thing, because only one of them lets `dev`
   generate its goreleaser config:

   | Repo | Own `.goreleaser.yml`? | What it needs |
   |---|---|---|
   | auth-proxy | no — `dev` generates it | `mise.toml` `outputs` → `.bin/<name>`. Nothing else: `.gitignore` heals itself on the first build (step 5a) and `.dist` comes with the generated config |
   | gsx | yes (`gsx`, `gsx-typebundle`) | `mise.toml` `outputs`, **plus** `dist: .dist` in its own `.goreleaser.yml` — required, not optional: `dev release` fails naming this until it is there |
   | gsxui | yes (`gsxui`, `stylegen`) | as gsx |

   The old `bin/` and `dist/` lines can stay in each `.gitignore`; they are
   harmless, and `Ensure` does not remove what it did not write.

   Then delete the stale `bin/`. Nothing in `dev` can do this for them, and
   nothing in `dev` should know their names — which is exactly why the `.dist`
   half is opt-in for a repo that owns its config, and automatic for one that
   does not.

## Checks

| | What is true | Why it is the thing to check |
|---|---|---|
| K1 | After step 1, `dev build .` in this repo writes `.bin/dev` and no `bin/` appears; `dev run` on a scaffolded repo executes it. | the rename actually moved, and `run` did not keep the old path |
| K2 | After step 2, `bin/dev release . --snapshot` — already part of `mise run check` — leaves `.dist/` holding the tar.gz files, `checksums.txt` and `packslip.sigstore.json`, and `packslip verify` passes on them. | goreleaser accepts a hidden dist dir and `--clean` handles it; packslip's `--out` followed. **Pre-verified end to end on 2026-09-17 (P1–P5); this is now a regression check, not an open question.** |
| K3 | A repo config with `dist: build/out` is signed from `build/out`, not `dist`. | Decision 2A fixed the latent bug rather than moving it |
| K4 | `mise run check` goes red if *either* `skills/dev/SKILL.md` or `.claude/skills/dev/SKILL.md` is not regenerated after step 1. | the skill is generated, and both checks still prove it |
| K5 | `git status` in this repo is clean after a full `mise run test` — no stray `bin/` or `dist/`. | the two `.gitignore` lines cover the new paths with the anchoring they had |
| K6 | A repo scaffolded by `dev init` after step 5 builds, runs and ignores `.bin`/`.dist` with no edits. | new repos are born migrated |
| K7 | A repo whose `.gitignore` still says `bin` / `/dist/` gains `.bin` on the first `dev build`, the binary lands in `.bin/`, `git check-ignore` confirms it, and a second build appends nothing. | **verified 2026-09-17** on a throwaway repo: `.gitignore` went from `bin\n/dist/` to the same plus a commented `.bin`, `.bin/tool` built and was ignored at `.gitignore:4`, and the second build left the file byte-identical |

## What was proven (2026-09-17, on this machine)

Every third-party behaviour this plan leans on was executed, not assumed.
goreleaser 2.x, packslip 1.2.0, mise 2026.9.10, macOS arm64.

| | Assumption | How it was tested | Result |
|---|---|---|---|
| P1 | goreleaser accepts `dist: .dist` | the **exact** config `goreleaserConfig()` emits, plus the one key, run as `goreleaser release --snapshot --clean` | **holds** — 6 targets built, archived to `.dist/`, checksums written |
| P2 | `--clean` handles a hidden dir | dropped a `STALE-MARKER` file in `.dist/`, re-ran | **holds** — marker gone, tree rebuilt, exit 0 |
| P3 | a relative `dist:` resolves against **cwd**, not the config file's directory | ran with the config in a scratch dir outside the repo — which is what `dev` does, since `newRelease` writes it to `os.CreateTemp` | **holds** — `.dist/` landed in the repo root. Had this gone the other way the whole approach was dead, because `dev`'s config lives in `/tmp` |
| P4 | `packslip create --out .dist` works | the real `createArgs` command line, throwaway key, 6 artifacts + the `skill/dev` resource | **holds** — wrote `.dist/packslip.sigstore.json` |
| P5 | `packslip verify` / `show` work from a hidden dir | as `snapshot()` calls them, `--allow-unlogged` | **holds** — `ok: ... signed by ... (1 of 6 artifact(s) checked, 1 resource(s))` |
| P6 | `go build -o .bin/dev` and `fnox exec -- .bin/dev` | the exact shapes at `stage/stage.go:147` and `:218` | **holds** — both exit 0 |
| P7 | mise tracks staleness through a dotted `outputs` | scratch project: run, re-run, touch source, re-run | **holds** — ran, then `sources up-to-date, skipping`, then ran again |
| P8 | the new ignore lines behave like today's | scratch repo, `git check-ignore -v` | **holds** — `.bin` catches nested `sub/.bin/`, `/.dist/` catches only the root, and `sub/dist/` stays tracked exactly as `/dist/` leaves it today |
| P9 | the latent `dist` bug is real | current `dev` + a repo config naming another dist | **reproduced** — see Decision 2 |

Baseline before and after: `mise run test` (lint + `go vet` + `go test` + both
skill checks + a real snapshot release) is **green**, and the tree carries no
leftovers from any of the above.

## Adjacent finding, not in scope

`newRelease` reads `.goreleaser.yml` and only that spelling. Two repos in the
workspace use `.goreleaser.yaml` (irgo, datapages) — **neither pins `dev`
today**, so nothing is broken, but if one ever did, `dev` would silently
ignore the repo's config and release from a generated one instead. A one-line
"try both spellings" would close it. Left out of this plan deliberately: it is
a different bug, and mixing it in would make the diff about two things.

## Risks that remain

- **Consuming repos break on the `dev` bump**, by design (Decision 1), and
  fixing them is **deliberately deferred**. The break is `dev run` failing to
  find `.bin/<name>`, plus, for gsx and gsxui, `dev release` failing until each
  adds `dist: .dist`. Both failures name their fix and neither can happen
  silently: a repo stays on its pinned `dev` until someone bumps it, so this
  lands when each repo chooses. Step 7's table is what they will need.
  Under Decision 2 the `.dist` half breaks gsx and gsxui's releases too, until
  each adds one line to its `.goreleaser.yml`. That is deliberate — one
  convention, stated by `dev` — and the failure names the fix rather than
  signing an empty directory.
- **Uncommitted in-flight work is in the tree** (`AGENTS.md`, `main.go`,
  `mise.toml`, `skills/dev/SKILL.md`, untracked `.claude/`) from outside the
  session that wrote this plan. Step 4 edits `mise.toml`, which that work also
  edits; land or stash it first so the two do not collide.
- **Out of scope, noticed in passing:** `.gitignore:2` ignores `/dev` — a
  stray root binary from a bare `go build`, left over from before the `bin/`
  task existed. It is unrelated to this plan and stays.
