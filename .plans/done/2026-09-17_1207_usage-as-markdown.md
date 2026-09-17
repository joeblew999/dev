# A verb's usage is markdown beside its code

**File:** `dev/.plans/2026-09-17_1207_usage-as-markdown.md` — refer to this plan by that name.

**Status:** DONE 2026-09-17 13:45 — `mise run test` exit 0 · **Created:** 2026-09-17 12:07 · **Revised:** 13:06
**Affects:** every `internal/*` package with a `const Usage`, `cli/command.go`
(a render path, an order field, a validator), and the scaffold's command
template and its test.
**Decisions taken:** markdown is the master and the terminal gets a flattened
render. Usage files sit beside the code they document. `Verb` and `Runner` are
unchanged; `Command` gains one additive field (`Order`).

## Decision and evidence

`Verb.Usage` is a `string` (`cli/command.go:23-26`). Where that string comes
from is the package's business, so `//go:embed usage.md` needs no change to
the `cli` contract, and `CheckSkill` keeps holding the manual to the verbs.
`main.go:26-33` already proved the pattern for `head.md` and `tail.md`.

Three things push usage the same way, each checked:

1. **Go raw strings are backtick-delimited, so usage can never contain a
   backtick.** The cost is in the template every new repo inherits,
   `internal/scaffold/files/cmd/__NAME__/main.go.tmpl:71-73`:
   `` ` + "`mise run __NAME__:run`" + ` ``.
2. **`<app>.fly.dev` and `<worker>-<binding>` are HTML tags to a markdown
   renderer.** Checked: unfenced, `<app>` survives into the HTML as a literal
   tag and a browser shows nothing. In backticks it escapes to `&lt;app&gt;`
   correctly. They are safe today only because `render()` fences everything.
3. **The manual is eight anonymous fenced blobs** (`skills/dev/SKILL.md`).
   Nothing marks where one package's verbs end and the next begins.

The obstacle is the fence in `cli/command.go:130`: markdown inside a code
fence renders literally, so markdown usage means dropping it — and then one
string is both terminal text and skill markdown. They pull apart, because the
4-space continuation indent the terminal wants means *code block* in markdown.

**Markdown's own list structure resolves it.** A verb is a list item: the
signature is its first line, the description its 2-space continuation.

```markdown
- `dev url DIR [--deployed[=BOOL]] [--env NAME] [--local URL]`
  print the URL to talk to: the deployed app in DIR when `--deployed`, else
  `--local` (default empty). A Fly app's is `<app>.fly.dev`.
```

Checked both directions. Rendered, the continuation stays inside the `<li>`
and the angle brackets escape. Flattened — drop `- `, re-indent continuations
by four, strip backticks and `**` — the output is byte-identical to today's
`app.Usage` terminal text, indents and blank lines included. The grouping is
carried by markdown semantics, not a convention invented for the flattener.

### Correction: the terminal output changes for three packages

Checked every usage block. There are two styles today, not one:

| Style | Packages | Ports to a list |
|---|---|---|
| Signature, then 4-space prose | `app`, `secrets`, `release`, `scaffold` | byte-identical |
| Signature padded, description on the same line | `stage`, `deps`, `session` | **changes** |

So "byte-identical terminal output" is only true for the first group. The
second group's description moves onto its own indented line. That is accepted,
for two reasons: one shape for every verb is the point of the exercise, and
the padded style is already broken — in `session`, `dev session verify
[--update]` overruns its 29-column pad, so its description starts in a
different column from its siblings'. The port fixes that.

`ownUsage()` (`cli/command.go:88-95`) is the same padded style, generated in Go
with `%-42s` and interpolating `c.Name` and the three paths. It cannot be a
static embed, so it becomes markdown built with `fmt` — but markdown it must
become, or the manual has one plain-text island.

### Correction: section order becomes visible, and it reads badly

`usages()` emits distinct usages in verb-name order (`cli/command.go:104-121`).
Checked against the rendered manual, that gives: **stage, app, deps, scaffold,
release, secrets, session, built-ins** — an alphabetical accident of whichever
verb name sorts first in each group (`build` < `delete` < `deps` < `init` <
`release` < `secrets` < `session` < `skill`). Today it hides inside anonymous
fences. With headings it becomes a table of contents, and `init` (the first
thing anyone does) sitting fifth is wrong.

So `Command` gains `Order []string`: verb names in manual order, each standing
for its group. Unlisted verbs follow in today's alphabetical order, so an
empty `Order` behaves exactly as now and no consumer repo is affected. `dev`
sets: `init, build, deploy, secrets, release, deps, session, skill`.

### How this reaches other repos: mise and packslip

Checked the ship path; **it does not change.** `newRelease` reads `skills/`
and makes one packslip resource per subdirectory,
`skill/dev=repo:skills/dev` (`internal/release/release.go:194-197`), passed as
`--resource` (`:344-346`). The `repo:` prefix means packslip serves it from
the source repo at the signed tag, not from a release artifact, so what
consumers get is the committed `skills/dev/SKILL.md`. They pin
`"packslip:github.com/joeblew999/dev"` and mise's `[settings.skills]
auto_sync` links it into `.claude/skills/dev`.

`internal/*/usage.md` is compiled into the binary by `go:embed`. It is not
under `skills/`, so it is never a resource. The resource stays one directory
holding one file; only the bytes inside that file change. `dev release` and
`createArgs` need no change at all.

**The freshness chain has a hole — checked, and it is not the one first
written here.** Because `repo:` ships what is committed, `SKILL.md` must be
current at the tag. `[tasks.build]` declares
`sources = ["**/*.go", "go.mod", "go.sum"]`, **with no `.md`**, so editing
only prose leaves mise reporting `sources up-to-date, skipping`.

The first draft of this plan said `CheckSkill` would then pass and the tag
would ship stale prose. **That is wrong.** `go test` recompiles the package
and re-embeds the *current* prose, so `TestSkill` fails naming all three
copies. A stale manual cannot reach a tag through `mise run test`.

What actually happens is a dead-end loop, reproduced end to end:

1. Edit `skill/head.md`. `mise run build` → `sources up-to-date, skipping`.
2. `go test` → `skills/dev/SKILL.md is stale; regenerate it with: dev skill`.
3. Run `dev skill`. It prints `wrote skills/dev/SKILL.md from the verbs' own
   usage` for all three copies — and writes the **old** prose, because the
   binary was never rebuilt. Success reported, nothing achieved.
4. `go test` fails identically. `mise run build` still skips.

The error names a fix that cannot work, and the tool reports success while
doing nothing. Escaping needs `mise run build --force`, touching a `.go`
file, or `go build` by hand. That is worse than a silent staleness: the
guard-rail fires correctly and then sends you in a circle.

**Fixed and verified** (2026-09-17 12:52). `skill/*.md` added to `sources`,
`skills/dev/SKILL.md` to `outputs`:

```toml
[tasks.build]
sources = ["**/*.go", "go.mod", "go.sum", "skill/*.md"]
outputs = [".bin/dev", "skills/dev/SKILL.md"]
```

Re-run: the prose edit rebuilt `.bin/dev`, `SKILL.md` picked up the change,
`go test ./...` green. Prose reverted, tree clean. `**/*.md` was rejected as
the fix — it would sweep in `README.md`, `AGENTS.md`, `.plans/*` and
`SKILL.md` itself, which `build` writes. When the ports land,
`internal/*/usage.md` joins `sources` the same way (step 1).


## DOD

- [x] Every `const Usage` in `internal/*` is a `usage.md` beside its code,
      embedded with `//go:embed usage.md`. No `const Usage` remains.
- [x] `ownUsage()` emits markdown, not padded columns.
- [x] Terminal output: byte-identical for `app`, `secrets`, `release`,
      `scaffold`; for `stage`, `deps`, `session` it matches a golden file
      reviewed and committed in step 6, and `session`'s alignment bug is gone.
- [x] A golden test pins the index (`dev` with no verb), so terminal
      rendering cannot drift silently again.
- [x] `skills/dev/SKILL.md` renders usage as markdown — a `###` heading per
      group, inline code, no fences around verbs — under head.md's `##
      Verbs`.
- [x] `cli.CheckUsage` exists, is called by `dev`'s `main_test.go` and by the
      scaffold's `main_test.go.tmpl`, and fails on: a code fence, a table, a
      numbered or nested list, a blockquote, a link or image, a setext
      heading, and **any `<...>` outside inline code**.
- [x] The supported subset is written in `skill/tail.md`, so it ships to every
      repo that pins dev.
- [x] Section order is `Order`'s, not alphabetical; `init` is first.
- [x] The scaffold template has no `` ` + "`x`" + ` `` concatenation left, and
      a generated repo's `go test` passes.
- [x] `[tasks.build]` names embedded prose in `sources` and every written
      copy in `outputs` — done 12:52/13:06 and both halves verified;
      `internal/*/usage.md` still to add when it exists.
- [x] The generated repo's `mise.toml` carries the same `sources` fix, or
      every scaffolded repo inherits the dead-end loop.
- [x] `dev skill --check`, `go test ./...` and `mise run check` green.

## Work

Ordered. Step 1 gates everything after it: port against a stale binary and
every check below is verifying the wrong bytes.

- [x] 1. `mise.toml` `[tasks.build]`: name the embedded prose in `sources` and
      every written copy in `outputs`. **Done 12:52/13:06** — `skill/*.md` in
      `sources` (loop reproduced, fix verified); all three `SKILL.md` copies in
      `outputs` (verified by deleting one). **`internal/*/usage.md` must be
      added before step 6**, when the first one exists — the only part left.
- [x] 2. `cli/usage.go`: `Flatten`, four rules only — heading, `- ` item,
      2-space continuation, paragraph — with a comment saying *why* the indent
      is added on render (four spaces in markdown is a code block). It must
      right-trim every line: `skills/dev/SKILL.md` is committed and hk's
      `trailing_whitespace` runs on it.
- [x] 3. `cli/usage.go`: `CheckUsage(t TB, c Command)`, exported beside
      `CheckSkill` so every consumer repo gets it, enforcing the DOD's ban
      list. The `<...>` rule is what turns a silent rendering bug into a test
      failure; it is the reason this exists, so write its test first.
- [x] 4. Transitional test: for each package, its markdown port must flatten
      to the existing const byte-for-byte (`app`, `secrets`, `release`,
      `scaffold`) or to the reviewed golden (`stage`, `deps`, `session`).
      Delete it once step 7 lands — the permanent net is the index golden.
- [x] 5. `Command.Order`; `usages()` honours it, unlisted verbs alphabetical
      after. Test that an empty `Order` renders exactly as today.
- [x] 6. Port `internal/stage` first: five verbs sharing one Usage, so it
      proves the dedup still holds with an embedded var, and it is one of the
      three whose terminal output changes — review and commit that golden
      before porting the rest. Add `internal/*/usage.md` to `sources` (step 1)
      with this first file.
- [x] 7. Port the rest: `app` (29 lines, the hardest), `secrets`, `scaffold`,
      `release`, `session`, `deps`. Then `ownUsage()`.
- [x] 8. `render()` emits usage unfenced; `index()` and the `UsageError`
      branch in `run()` both emit `Flatten(v.Usage)` — one call site each,
      through the same function, or the two terminal paths drift.
- [x] 9. Set `dev`'s `Order`; regenerate; read the manual top to bottom and
      confirm it reads as a manual, not a dump.
- [x] 10. Scaffold: `usage.md`, `head.md`, `tail.md` beside `main.go.tmpl` as
      `.tmpl` files, embedded by the generated `main.go`;
      `main_test.go.tmpl` calls `CheckUsage`; the generated `mise.toml` gets
      the step 1 `sources` fix too. Run `dev init` into a scratch dir and
      `go test` inside the generated repo — a template that compiles here but
      not there is the failure mode.
- [x] 11. Document the subset in `skill/tail.md`. `dev skill`,
      `mise run check`, record results here.

## Results

`mise run test` (lint + vet + tests + a signed snapshot release) exits 0.
`dev init` into a scratch dir produces a repo whose `go test` passes and whose
manual and terminal output are both correct — checked by compiling the
generated command against this working tree.

Three things the work turned up that the plan had not predicted:

- **`sources` was short by more than markdown.** Beyond `skill/*.md` and
  `internal/*/usage.md`, `cli/own_usage.md` and the whole embedded template
  tree `internal/scaffold/files/**/*` were missing — `.tmpl` is not `.go`, so
  editing a scaffold template never rebuilt the binary either. That gap
  predates this work. `sources` now names every `go:embed` input, found by
  grepping the directives rather than by guessing.
- **`skill/tail.md` already shipped a broken placeholder.** `NAME<TAB>OWNER`
  was written without backticks, so every rendered copy of the manual showed
  "NAMEOWNER" and lost the fact that the lines are tab-separated. It was
  invisible because the verbs around it were fenced. `CheckUsage` is the rule
  that stops it recurring; this instance was found by scanning and fixed by
  hand, since prose outside usage is not covered.
- **`AGENTS.md` had trailing whitespace** that `mise run test` had never
  caught, because lint only runs in the full gate and the last commits used
  `check`. Fixed with `hk util trailing-whitespace --fix`.

## Follow-up: the stale-binary class, fixed 2026-09-17 14:05

The `sources` fix stopped mise skipping the rebuild, but the same root cause
had two more faces, both found by running the round trip by hand:

- `<cmd> skill` from a binary behind its sources rewrote every copy from the
  old embedded prose and printed `wrote ... from the verbs' own usage`.
- `<cmd> skill --check` compared that old render against equally old files and
  printed `up to date`, exit 0, while `go test` correctly called it stale.

Neither is visible from the output, which is what made them dangerous: the
tool reports success for work it did not do. `go test` was the only thing that
could not be fooled, because it recompiles.

`cli/stale.go` now refuses both. Before writing or checking, a command asks
whether any `.go` or `.md` under the repo root has changed since the binary
was built, and if so names the file and stops. Two things keep it honest:

- It applies only to a binary **inside the repo**, which is one this repo
  built. A release installed by mise carries prose fixed at its pinned
  version, which nothing in a consumer's repo can make stale; asking there
  would refuse every run.
- Output and documents (`skills/`, `.claude/`, `.agents/`, `.plans/`,
  `.dist/`, `.bin/`) are not sources. Naming one too few costs a needless
  rebuild; naming one too many costs a wrong answer, so it errs towards
  refusing.

Verified: the edit-without-rebuild case now fails both paths naming
`internal/deps/usage.md`; `mise run build` clears it; a copy of the binary run
from outside the repo does not refuse; `mise run test` exit 0; a scaffolded
repo's `go test` still passes.

## Follow-up: unported repos, fixed 2026-09-17 14:40

Asked whether every repo now knows the system, and checked instead of
assuming. Simulated an existing repo — a `cli.Command` with the old plain-text
`Usage` const — against this working tree. Unfenced, its manual was quietly
made worse by the upgrade: markdown collapsed the hand-aligned columns to
single spaces, ran both verbs into **one paragraph**, and ate
`<app>.fly.dev` as an HTML tag. Nothing failed; the manual just got worse.

That is the same silent-degradation class as the rest of this plan, so
`render()` now fences usage that does not use the markdown shape (no heading,
no `- ` item), exactly as every usage was fenced before. Verified: the
unported command's manual keeps its alignment and its `<app>.fly.dev`, its
terminal output is byte-identical, and it still gets the markdown
`### The tool itself` section. `Flatten` was already a no-op on legacy text,
in both the column and indented styles; there is now a test saying so.

A repo therefore ports when it chooses, not when it bumps a pin. The
migration is written into `skill/tail.md`, so it ships with the skill.

## Issues raised

- **`Flatten` and `CheckUsage` enter `cli`, the public API**, so every repo on
  the stack inherits them. Four rules and a ban list are what stop `Flatten`
  becoming a markdown engine. Adding table or link support later is a decision
  to take deliberately, not a patch.
- **Steps 2-9 must not ship without 10.** `main.go` already embeds its prose
  while the template it ships still concatenates around backticks; until 9
  lands, every new repo inherits the older pattern and the step 1 loop.
- **`skill/` (source prose) and `skills/` (what ships) differ by one
  letter**, and `release.go` resolves the shipped one by the literal string
  `"skills"`. Nothing breaks today, but the pair is a trap for anyone
  moving files between them. Still open.
- **`CheckUsage` covers usage, not `head.md`/`tail.md`** — closed 14:20.
  `CheckUsage` now checks a command's Head and Tail too, and the two are held
  to *different* rules, because they are read differently. A verb's usage is
  rendered twice, as markdown in the manual and as flattened text in a
  terminal, so it must stay inside the subset `Flatten` reads. Head and Tail
  only ever reach the manual — checked: `Flatten` is applied at
  `cli/command.go:79` and `:171`, both usages only — so fences, tables, links
  and emphasis are correct there and are not refused. What applies to both is
  the angle-bracket rule, since that is a rendering bug rather than a
  flattening one, and prose is exactly where it bit. Frontmatter is blanked
  before the check, keeping its lines so reported line numbers still count
  from the top of the file: it is YAML the renderer never sees, and its `---`
  would otherwise read as a setext heading. Verified by reintroducing the
  original `NAME<TAB>OWNER` into tail.md — the test named the file, the line,
  the offender and the fix.
- **The generated repo cannot be tested without a published dev.** Verifying
  the scaffold needed a local `replace` added by hand. A task that does that
  for a scratch repo would make `dev init` testable in CI.
- **`outputs` named one of three written files — closed 13:06.** `dev skill`
  writes `skills/`, `.claude/skills/` and `.agents/skills/`, and all three are
  tracked; only mise's own generated skill links are gitignored. All three are
  now in `outputs`, verified by deleting the `.agents/` copy and watching
  `mise run build` rebuild instead of skipping.
- **hk's `trailing_whitespace` runs on every file, `.md` included.** That
  is a guard, not a risk: it holds the markdown sources and the rendered
  manual to the same rule. `Flatten` must right-trim so it never emits
  what the linter then rejects.
- **`session`'s alignment bug is evidence, not a side quest.** It is what
  hand-maintained column padding costs, and it argues the port is overdue.

## Not in this plan

- The `AGENTS.md` rules move into `skillHead`/`skillTail` — that is
  `2026-09-17_1055_agents-rules-ride-the-skill.md`, independent, either order.
- Any change to `Verb` or `Runner`. `Command` gains `Order` and nothing else.
- Localising usage — see `2026-09-17_0852_i18n-into-dev.md`.
