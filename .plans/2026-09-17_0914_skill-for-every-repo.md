# dev's verb system becomes a library — every command that uses it gets its skill for free

**File:** `dev/.plans/2026-09-17_0914_skill-for-every-repo.md` — refer to this plan by that name.

**Status:** in progress, steps 1–5 done, committed and pushed 2026-09-17
(`ddb1fad`); steps 6–7 not started, and both are outside this repo · **Created:** 2026-09-17 09:14 · **Rewritten:** 2026-09-17
**Finished:** — · **Results recorded:** 2026-09-17 (K1–K6, K8–K9; K7 pending step 6)
**Affects:** this repo (`internal/cli` becomes importable, `main.go` loses its
dispatch loop and its generator) and every command in a repo that pins `dev` —
nine today, in auth-proxy, gsx and gsxui — plus every command `dev init` writes.
**Decisions needed before starting:** none. The Product Owner set the unit and
the source (2026-09-17): the unit is a *command*, the source is its *verb table*,
and nothing is rendered from mise tasks at this stage.

**Rewritten 2026-09-17.** The first draft made the repo the unit and mise tasks
the source, and grew three decisions nobody could follow. The Product Owner's
framing is simpler and right: a using repo has commands; if a command uses the
verb system `dev` uses, it gets a skill the way `dev` does. So the thing to
build is not a generator that knows about repos — it is the verb system itself,
made importable, with the generator inside it.

## Decision and evidence

`dev`'s manual is rendered from its verb table, so it cannot drift from the
binary. That is the good idea. Today it is welded to `dev`:

| `main.go` | Lines | What it is |
|---|---|---|
| `main()` | 57–90 | the dispatch loop: no verb → index; `version`; `skill`; lookup; usage error → exit 2; error → exit 1 |
| `index()` | 93–112 | the verb list the binary prints |
| `skill()` | 117–162 | the generator and `--check` |
| `skillHead`, `skillTail` | 164–205 | `dev`'s own prose |

Only the last row is `dev`'s. The other three are what *any* command with a
verb table needs and none can reach, because `internal/cli` — the shape they
are built on — is `internal`. And `skill` and `version` are handled at
`main.go:64` and `:68` *before* the table is consulted, so they are in neither
`index()` nor the manual: `skills/dev/SKILL.md` never mentions `dev skill`.

**The shape already exists; it is just not importable.** `internal/cli` holds
`Runner`, `UsageError`/`Usagef`, `Flags`, `Bool`, `DirAnd`, `ParseInterleaved`,
`Confirm` — five stdlib imports, nothing else — and eleven files in `dev` build
on it. Every one of `dev`'s packages is already "a `Usage` const and a
`Run(verb, args, stdout, stderr) error`". A command in another repo that wrote
the same would be one `map` away from having a manual. It cannot, because the
map's loop and the renderer live in `main.go`.

**Where the commands are.** Three repos pin `dev` and release through
`dev release`, which already ships every `skills/<name>/` directory as a
packslip resource (`release/release.go:178-181`). So a skill a command writes
into `skills/<name>/` travels to that tool's consumers with no new plumbing.

| Repo | Modules | Commands | Shape today |
|---|---|---|---|
| gsxui | one, at the root | `gsxui`, `stylegen` | `stylegen` is verb-shaped by hand (`os.Args[1] == "port"`, then flags) |
| gsx | one, at the root | `gsx`, `gsx-examples`, `gsx-typebundle` | `gsx` is `gen.Main()`, a library API other binaries extend; the other two are flags |
| auth-proxy | one **per command** | `proxy`, `gui`, `mock-upstream`, `bench` | three HTTP servers and a bench: flags, no verbs |

None imports `dev` today. "Use the verb system" is therefore not free per
command — it is a port of that command's CLI — but the **skill** is free once a
command has: it writes its verbs and its prose, and `<name> skill` exists. What
this plan makes true is the second half.

**The scaffold's own command is a server.** `dev init` writes an HTTP server
that answers `/health` — `flag.Parse`, `ListenAndServe`, no verbs. A server has
no verb list to render, and `dev run cmd/<name>` runs the binary with no
arguments, which under a verb table would print the index and exit 2. One
field fixes both: a command may name a **default verb**, run when there is
none. The scaffold's server becomes `serve [--addr]` with `Default: "serve"`;
`stylegen`'s flags-by-default become the same. `dev` names no default and
prints its index, as now.

**Staleness is checked by `go test`, not by a task.** `dev check DIR` already
runs `go test ./...` in the command's directory. The library ships one test
helper; the scaffolded `main_test.go` calls it in one line; a changed usage
string with an unregenerated skill is a failing test, in every repo, with no
change to `stage` and no new task. `dev`'s own `bin/dev skill --check` in
`mise.toml` goes the same way — `dev` uses the path everyone else does.

**The library stays stdlib-only, so it must not find the repo through git.**
`internal/gitrepo` shells out (`os/exec`), and a command that ships as a Worker
builds its `package main` for wasm too. `cli` therefore finds the repo root by
walking up to `mise.toml` — the file every repo on this stack has at its root
and nowhere else — and writes `<root>/skills/<name>/SKILL.md`. Check K4 holds
it to the standard library.

**The skill reaches the repo's own agent directories too.** Nothing links it
there today: packslip links a *pinned tool's* skill and `session.toml` vendors a
*pinned upstream's*, so a repo's own `skills/<name>/` is invisible to its own
sessions — which is how this repo went a day with its own manual out of
context. `dev`'s `mise.toml` writes a second copy by hand as of 2026-09-17; the
verb makes that the rule. `<name> skill` writes `skills/<name>/SKILL.md` (what
ships) **plus** `.claude/skills/<name>/SKILL.md` (Claude Code) **and**
`.agents/skills/<name>/SKILL.md` (Copilot), one render, three writes, and
`CheckSkill` holds all three. Real directories, not symlinks, because
mise's `[settings.skills] prune` leaves real directories alone. Committed, like
the vendored copies beside them.

**Regeneration is a side effect of building, not a step to remember.** Every
stage a repo runs — `<cmd>:build`, `:run`, `:check` — goes through
`dev build DIR`, which already reads what the directory holds
(`stage.Inspect`: gsx, wrangler, wasm). A command that imports `dev/cli` is
one that has a manual; after building it, `dev build` runs `bin/<name> skill`,
and both copies are current before the developer types the next thing. Claude
Code reads `.claude/skills/` live — proved 2026-09-17, when writing
`.claude/skills/dev/SKILL.md` put the skill into a running session with no
restart — so a changed verb is in the repo's next prompt. `TestSkill` stays as
the guard for a commit made without a build. This is what "other repos get it
too" means: they change a command, its skill follows, their Claude has it.

**`dev` becomes importable, for the first time.** This is the same consequence
the i18n plan records (`2026-09-17_0852_i18n-into-dev.md`, "the constraint the
tool has never had") and the two compound: a command importing `dev/cli` also
downloads whatever else the module carries. Concretely: `dev`'s `go 1.27.1`
becomes a floor (auth-proxy's commands declare `go 1.27.0`); `cli`'s API becomes
a contract, versioned by the tool's tags, where today `internal/cli` can change
freely; and the eleven in-repo importers are a mechanical rename.

## What the library is

```go
package cli // github.com/joeblew999/dev/cli — was internal/cli; everything it had, plus:

type Verb struct {
    Run   Runner
    Usage string
}

type Command struct {
    Name    string          // the binary's name; skills/<Name>/SKILL.md
    Verbs   map[string]Verb // what main.go's table is today
    Default string          // verb run with no arguments; "" prints the index
    Version string          // what `<name> version` prints
    Head    string          // the skill's frontmatter and prose before the verbs
    Tail    string          // the prose after
}

func Main(c Command)                       // main.go:57-90 and index(), for any command
func CheckSkill(t testing.TB, c Command)   // fails when skills/<Name>/SKILL.md is stale
```

`Main` provides `skill [--check]` and `version` itself, lists them in the index
with the rest, and renders them into the manual — which fixes the one thing
that was nuts before anything is shared. `skill` writes two files from one
render, `skills/<Name>/SKILL.md` and `.claude/skills/<Name>/SKILL.md`; `--check`
and `CheckSkill` hold both. A command is then:

```go
var app = cli.Command{
    Name:    "stylegen",
    Default: "generate",
    Verbs:   map[string]cli.Verb{"generate": {run, usage}, "port": {run, usage}},
    Head:    head, Tail: tail,
}

func main() { cli.Main(app) }
```

and its test is `func TestSkill(t *testing.T) { cli.CheckSkill(t, app) }`.

## Work

1. **`internal/cli` → `cli`.** Move the package; update the eleven importers.
   Nothing else changes in this step; `mise run check` proves it.
   **Done 2026-09-17** (working tree): `cli/cli.go` + `cli/command.go` (+ tests),
   no `internal/cli` references remain, `go vet ./...` and `go test ./...` green.
2. **`Verb`, `Command`, `Main`, `CheckSkill`.** `Main` is `main.go:57-112`
   generalised: the same exit codes, the same index, `skill` and `version`
   provided rather than special-cased. The renderer is `main.go:117-162` with
   both paths found by walking up to `mise.toml`: `skills/<name>/SKILL.md` and
   `.claude/skills/<name>/SKILL.md`, one render, two writes. Every error names
   its fix — a stale copy says which one and `regenerate it with: <name> skill`.
   **Done 2026-09-17** (working tree): `cli/command.go` holds all of it; `TB`
   interface keeps `testing` out of linked binaries.
3. **`dev` becomes the first command.** `main.go` keeps its verb table (now
   `map[string]cli.Verb`), `skillHead`/`skillTail` as `Head`/`Tail`, the ldflags
   `version`/`pubkey` hand-off to `scaffold`, and `func main() { cli.Main(app) }`.
   `index()`, `skill()` and the two special cases are deleted. `main_test.go`
   gains `cli.CheckSkill`; `mise.toml`'s `check` drops both `bin/dev skill
   --check` lines because `go test` now does it, and the `skill` task's second
   `--out=.claude/skills/dev/SKILL.md` line (added by hand 2026-09-17) goes
   because the verb writes both. Regenerate both copies and commit them.
   **Done 2026-09-17** (working tree, uncommitted): `main.go` is 106 lines;
   `main_test.go` is `cli.CheckSkill(t, dev)`; `mise.toml` `check` has no
   `skill --check` lines and `build` runs `.bin/dev skill`. Both copies
   regenerated and byte-identical. Not yet committed.
4. **`dev build` regenerates it.** `stage.Inspect` learns whether the module
   in DIR imports `github.com/joeblew999/dev/cli` (`go list -deps`, beside the
   reads it already does for gsx, wrangler and wasm); when it does, `Build`
   runs `bin/<name> skill` after the binary is written, and both copies are
   rewritten. Nothing runs for a command that does not use the verb system, so
   a build never touches a hand-written skill. `dev`'s own `build` task does
   the same for itself, and its `skill` task survives only as a name for it.
   **Done 2026-09-17** (working tree): `stage.go` `Dir.CLI` + `importsCLI` +
   post-build `bin/<name> skill`; `TestInspectSeesACLICommand` covers it.
5. **The scaffold writes a verb-shaped command.** `cmd/__NAME__/main.go.tmpl`
   becomes `serve [--addr]` under `cli.Main` with `Default: "serve"`, a
   `Head`/`Tail` stub naming the command, and `go.mod.tmpl` requires
   `github.com/joeblew999/dev`. `main_test.go.tmpl` keeps `TestHealth` and adds
   `TestSkill`. `dev init` also runs the generator once, so
   `skills/__NAME__/SKILL.md` and `.claude/skills/__NAME__/SKILL.md` exist from
   the first commit: the first `dev check` is green, the first release ships a
   skill, and the first Claude Code session in the repo has it in context.
   `.gitignore.tmpl` ignores `.claude/skills/dev` because mise links that one;
   it must not ignore the repo's own `.claude/skills/__NAME__`.
   **Done 2026-09-17** (working tree): templates are verb-shaped; `Init` runs
   `go run . skill` in the new command once its module resolves (offline it
   names `go mod tidy && go run . skill` as the fix); `TestInit…` asserts the
   generator ran. `.gitignore.tmpl` ignores only `.claude/skills/dev` (+ hk
   links), never the repo's own skill. Two fixes landed while proving this:
   `Run` accepted no `DIR .` with flags after it (rewritten to branch on
   first-arg shape), and the probe used `--pin v0.4.4` where the template
   wants `0.4.4` (`v__PIN__`).
6. **Prove it on a command that is not `dev`.** gsxui's `stylegen`: already
   verb-shaped by hand, one root module (one `require`), released through
   `dev release .` so its `skills/` ships. Port its `port` verb and its default
   flags to a table with `Default`; add `TestSkill`; the next
   `mise run stylegen:build` writes `gsxui/skills/stylegen/SKILL.md` beside the
   hand-written `skills/gsxui/` and `gsxui/.claude/skills/stylegen/SKILL.md`
   beside the linked `dev` one — and gsxui's own Claude sessions have it from
   that build on. The port lands in gsxui's own `.plans/` when it starts,
   pointing here.
   **Not started.** Recon 2026-09-17: `gsxui/cmd/stylegen/main.go` dispatches
   `os.Args[1] == "port"` by hand with default flags otherwise — the exact
   `Default: "generate"`-shaped port the plan describes; gsxui is one root
   module at `go 1.26.1`, so the `require` will raise its floor to 1.27.1 (K8).
7. **Release and pin.** `mise run release <version>` here; gsxui bumps its pin
   and adds the `require`. Nothing lands in gsxui until the tool is released.
   **Not started.**

## What step 6 now inherits (2026-09-17, later)

The usage-as-markdown work (`.plans/done/2026-09-17_1207_usage-as-markdown.md`)
landed after steps 1–5, so the port `stylegen` does is not quite the one
written above:

- A verb's usage is a `usage.md` beside the code, embedded with `//go:embed`,
  not a `Usage` string const. `Head`/`Tail` likewise — `head.md`, `tail.md`.
- The command's test calls `cli.CheckUsage` beside `cli.CheckSkill`.
- gsxui's build task must name that markdown in `sources` and the manual's
  three copies in `outputs`, or editing prose leaves the binary stale while
  mise reports it fresh.
- `Command.Order` exists if `stylegen` ever has enough verbs to want one.

None of this blocks the port; `stylegen` can keep plain-text usage and it will
be fenced in the manual exactly as before. It is the shape to port *into* when
the port happens.

**Step 7 is the gate.** gsxui cannot `require github.com/joeblew999/dev` until
a version is published — a real `mise run release <version>`, not a snapshot —
because the Go module proxy resolves the require from a pushed tag. Nothing in
step 6 lands in gsxui before that.

## Checks (record results in the plan)

| # | Check | Why | Result 2026-09-17 |
|---|---|---|---|
| K1 | After step 1: `mise run check` green with nothing but import paths changed. | the rename is mechanical or it is not; find out before anything is built on it | **Pass.** No `internal/cli` references remain; `go vet ./...` clean, `go test ./...` all green. (Full `mise run check` not run — it needs fnox/network for the snapshot release; vet+test are its code gates.) |
| K2 | After step 3: `bin/dev` with no arguments lists `skill` and `version`; the regenerated `skills/dev/SKILL.md` differs from today's by exactly those two entries. | the one thing that was nuts is fixed, and nothing else in the manual moved | **Pass on the index; differ on the diff.** Index lists `dev skill [--check]` and `dev version` exactly once each. The regenerated manual contains those two entries, but also carries unrelated drift committed since (`.bin`→`bin` renames, release/usage prose) — the working tree was already ahead of HEAD before this plan. |
| K3 | `wc -l main.go`. Before, measured 2026-09-17: **205**. After: the table, `Head`/`Tail`, the hand-off and a one-line `main` — expect about 95. | the dispatch loop and generator left; record that they did | **106 lines** (expectation ~95; the table itself is 20 lines). Dispatch loop, `index()`, `skill()` gone. |
| K4 | `go list -deps ./cli \| grep -v '^github.com/joeblew999/dev'` is the standard library only, and `GOOS=js GOARCH=wasm go vet` — which `dev check` already runs on a Worker command — passes on the scaffolded command. | a Worker's `package main` links `cli` into its wasm; it must cost nothing there | **Pass both halves.** `cli` deps are stdlib-only (no dotted third-party imports). `GOOS=js GOARCH=wasm go vet` passes on the scaffolded `widget` command in the `/tmp/init-probe` repo. |
| K5 | In a repo from `dev init`: change a verb's usage string, run `dev check cmd/<name>` without building — red, naming `<name> skill` as the fix; run `dev build cmd/<name>` — both copies rewritten, `git diff` shows the change in each, check green. | the drift property that only `dev` has today, now with `go test` as the guard and `dev build` as the fix, in a repo that is not `dev` | **Pass**, proved in `/tmp/init-probe/widget` (local `replace` to this checkout): usage edit → `go test ./cmd/widget/` red naming `widget skill`; `dev build cmd/widget` rewrote both copies; check green. |
| K6 | In `dev` after step 4: edit a usage string, `go test .` alone is red naming `dev skill`; `mise run check` is green because its `build` regenerates first and `git diff` then shows both copies changed. `bin/dev skill --check` is gone from `mise.toml`. Recorded 2026-09-17: exactly this. | `dev` keeps the guarantee by the shared path — the test guards, the build heals — and never by a task of its own | **Pass.** Editing the `version` usage in `cli/command.go` → `go test .` red on both copies naming `dev skill`; `go build` + `dev skill` healed both; green after. `mise.toml` `check` has no `skill --check`; `build` runs `.bin/dev skill`. Tree restored pristine after. |
| K7 | In gsxui after step 6: `stylegen` with no arguments does what it did; `dev release . --snapshot` emits `skill/stylegen=repo:skills/stylegen` beside `skill/gsxui=…`; `skills/gsxui/SKILL.md` is byte-identical. | the default verb keeps the old behaviour, the new skill ships, the hand-written one is untouched | **Pending** — step 6 not started. |
| K9 | In a repo from `dev init`, and in gsxui after step 6: `.claude/skills/<name>/SKILL.md` exists, is byte-identical to `skills/<name>/SKILL.md`, and a fresh Claude Code session there lists the skill; `mise install` (which runs `mise skills sync` with `prune`) leaves it in place. | the repo's own sessions read its own manual with no release and no pin — the thing that was missing on 2026-09-17 | **Half pass.** In `/tmp/init-probe/widget`: both copies exist, byte-identical, and `git check-ignore` confirms neither is ignored (committable). Fresh-session listing and `mise install`+`prune` survival not yet exercised. gsxui half pending step 6. |
| K8 | Every `go.mod` that gains `require github.com/joeblew999/dev` has `go` ≥ `dev`'s 1.27.1; list which had to rise (auth-proxy's `1.27.0` will, when its turn comes). | the floor the import brings; make it visible rather than discovered | **No rises yet.** `go.mod.tmpl` already declares `go 1.27.1`; the probe repo's module is 1.27.1. Known future rise: gsxui root module is `go 1.26.1` (step 6 will raise it). auth-proxy's `1.27.0` still to come. |

## Acceptance

- [x] `github.com/joeblew999/dev/cli` is importable; `main.go` has no dispatch loop, no `index()`, no `skill()`.
- [x] `dev`'s manual documents `dev skill` and `dev version`.
- [ ] A command in a repo that is not `dev` renders its own skill from its own verb table with no `dev`-specific code in it — `stylegen`. (Proved with scaffolded `widget` instead; `stylegen` is step 6.)
- [x] A repo from `dev init` gets a verb-shaped command whose skill ships with its first release and whose `check` fails when the skill is stale. (Proved in `/tmp/init-probe`; `release/` already ships every `skills/<name>/` — `release.go:190-193`.)
- [x] Every command's skill is in its own repo's `.claude/skills/<name>/` from the first commit, stays there through `mise install`, and is rewritten by every `dev build` of that command — a changed verb reaches that repo's Claude with no step taken. (First-commit + rewrite proved; `mise install`+`prune` survival assumed from real-directory rule, not yet exercised.)
- [ ] K1–K9 recorded above. (K7 pending; K9 half pending.)

## Not in this plan

- **Rendering from mise tasks.** Explicitly not at this stage (Product Owner,
  2026-09-17). The verb table is the source; a repo's tasks are documented by
  `mise tasks`, live.
- **Porting the other eight commands.** gsx's `gsx` dispatches through
  `gen.Main()`, an API other binaries extend — whether it adopts `cli` is gsx's
  call, not a mechanical port. auth-proxy's three servers and `bench`, gsx's two
  flag tools and gsxui's `gsxui` can each take the scaffold's shape
  (`Default: "serve"` or the like) once the library exists; each is a line in
  its own repo's plans, not this one.
- **The skill-gate's coverage map** (`scaffold/files/.claude/hooks/skill-gate.tmpl:64-66`)
  is a second hand-kept list of skills in every scaffolded repo. Untouched here.
