# Modern Go, by the JetBrains guidelines: go fix for what it can, hands for the rest

**File:** `dev/.plans/2026-09-17_2120_modern-go-by-the-guidelines.md` — refer to this plan by that name.

**Status:** DONE 2026-09-17 · **Created:** 2026-09-17 21:20 · **Finished:** 2026-09-17 21:40
**Affects:** this repo only. `cli/` changes are inside function bodies; no verb,
flag or exported shape moves, so nothing that builds on `cli` is touched.
**Decisions needed before starting:** none. The Product Owner set the source
(2026-09-17, AGENTS.md line 10): follow
https://github.com/JetBrains/go-modern-guidelines, its way, and use `go fix`
extensively.

## What "its way" is

The guidelines repo is 54 rules in `internal/guidelines/guidelines.json`,
rendered as `FEATURES.md`, served by a small CLI. Its skill says: before
editing a Go file run `list --file-path <file>`, read the whole list, treat it
as authoritative, and before skipping a rule that seems to apply run `explain
<id>`. Skip only when it would not compile, would change behaviour, or clearly
does not match the code.

Built from the clone and pointed at this repo's `go.mod` (`go 1.27.1`), `list`
returns all 54. Each rule carries a `modernizer` flag: 25 have one, and those
are exactly what `go fix ./...` under go1.27.1 rewrites. The other 29 are hand
work, found by grepping for each rule's "before" shape.

## Evidence: the 54 rules against this tree

**25 with a modernizer — done by `go fix`, commit `092524e`.** `go fix -diff
./...` prints nothing. (any, range_over_int, loopvar_capture, min_max,
slices_contains, slices_sort, slices_clip, slices_clone, maps_copy,
bytes_clone, bytes_cut, strings_cut, strings_cut_prefix_suffix,
strings_split_seq, strings_bytes_cut_last, errors_as_type, new_expression,
sync_waitgroup_go, testing_t_context, testing_b_loop, json_omitzero,
fmt_appendf, atomic_types, reflect_type_for, promoted_field_literals.)

**29 by hand.** Every one was grepped for; here is what the tree holds.

| rule | impact | where | change |
|---|---|---|---|
| `errors_is` | Critical | `internal/session/fetch.go:28` `err == io.EOF` | `errors.Is(err, io.EOF)` |
| `slices_sorted` + `maps_keys_values_iter` | Medium/High | `cli/command.go:190` `sortedVerbs` | `slices.Sorted(maps.Keys(verbs))` |
| 〃 | | `internal/cloudflare/url.go:203` keys of `env` | 〃 |
| 〃 | | `internal/session/pins.go:72` `names()` | 〃 |
| 〃 | | `internal/session/verify.go:202` plugin names | 〃 |
| 〃 | | `internal/session/lock.go:90` `sortedFiles`, minus the lock | `slices.DeleteFunc(slices.Sorted(maps.Keys(files)), …)` |
| `cmp_or` | High | `internal/session/verify.go:93` `was` | `cmp.Or(recordedBy, "an unrecorded version")` — constant fallback |
| 〃 | | `internal/cloudflare/delete.go:26` `name` | `cmp.Or(name, workerName(cfg, env))` — `workerName` is a pure map lookup |
| 〃 | | `cli/command.go:129` `usage` | `cmp.Or(one(…path), one(…verb))` — `one` is a pure lookup and render |

Skipped, with the rule's own reason (`explain` read for each):

| rule | where | why not |
|---|---|---|
| `cmp_or` | `internal/release/keys.go:33` `seed, _ = fnox.Get(…)` | the fallback runs a subprocess: expensive and a side effect, which the rule names as the wrong case |
| `cmp_or` | `internal/fly/fly.go:142` | the fallback is a call that can fail; three statements, not a chain of values |
| `cmp_or` | the other 17 `if x == ""` sites | they `return` or error; not a fallback |
| `context_timeout_deadline_cause` | `internal/session/verify.go:145`, `internal/stage/browser.go:152` | the rule is for callers that inspect the *reason*; both only ask whether the deadline passed |
| `maps_delete_func` | `internal/session/settings.go:64` | the loop ranges a key list and writes as well as deletes; not a predicate over the map |
| `json_v2` | the four `encoding/json` imports | the rule itself: leave existing code unless migration is explicitly requested; it was declined today |

No site in the tree: `generic_methods`, `stdlib_uuid`, `url_clone`,
`slices_collect` (every collect here also sorts), `time_tick_gc`,
`http_servemux_patterns`, `clear` (already used), `slices_index`,
`slices_index_func`, `slices_sort_func`, `slices_max_min`, `slices_reverse`,
`slices_compact`, `maps_clone`, `sync_once_func`, `sync_once_value`,
`context_after_func`, `context_cancel_cause`, `errors_join`, `strings_clone`,
`time_until`, `time_since`.

## AGENTS.md

It has two `## Modern Go` sections tonight: the Product Owner's pointer at the
top (line 8) and the earlier bullet list (line 94) that restates rules the
guidelines already hold. One source per fact: the section points at the
guidelines and how to run them here, and keeps only what the guidelines do not
say — that a feature that seems too new is checked with `go doc`, that the json
imports are v1 on purpose, that `NewTestServer` needs `Start()` when the code
under test brings its own client.

## DOD

- [x] `go fix -diff ./...` prints nothing, after the hand edits as well as before.
- [x] Each of the 29 hand rules has a row above: applied with file:line, no
      site, or skipped with the rule's own reason.
- [x] `mise run check` and `mise run lint` green; the rendered manual unchanged.
- [x] AGENTS.md has one `## Modern Go`, pointing at the guidelines, not restating them.
- [x] Results recorded here; plan in `done/`; commit of named files.

## Work

One file at a time, exact match, `go build ./...` after each.

- [x] 0. Commit green: `092524e`.
- [x] 0b. Install the tooling as README#claude-code says (Product Owner,
      AGENTS.md line 20). Done through the `claude plugin` CLI, the
      non-interactive form of the two slash commands: `claude plugin
      marketplace add JetBrains/go-modern-guidelines`, then `claude plugin
      install modern-go-guidelines@goland-claude-marketplace`. It is user
      scope, v1.1.1, enabled — `claude plugin list` shows it. Its skill's
      first run `go install`s the CLI from a temp directory under `~/.cache`,
      which failed here: `go` came only from mise shims, which resolve from
      the current directory, and no repo pins one there. mise's own error
      named the fix and it is the least that works: `mise use -g go@1.27.1`,
      a global default that a repo's own pin overrides. The CLI is now at
      `~/.cache/go-modern-guidelines/v0.1.1/go-modern-guidelines` and `list`
      and `explain` answer through the skill's script.
- [x] 1. `errors.Is` — `internal/session/fetch.go`.
- [x] 2. `slices.Sorted(maps.Keys(m))` — `cli/command.go`, `internal/cloudflare/url.go`,
      `internal/session/pins.go`, `internal/session/verify.go`, `internal/session/lock.go`.
- [x] 3. `cmp.Or` — `internal/session/verify.go`, `internal/cloudflare/delete.go`, `cli/command.go`.
- [x] 4. `go fix -diff ./...` after the hand edits: silent on the first pass.
- [x] 5. AGENTS.md: one Modern Go section. The Product Owner had already
      removed the restating bullets; what remained was two headings, the
      pointer, and a trailing space hk would have refused.
- [x] 6. `mise run check`, `mise run lint`, `mise run dead`; results here; move to `done/`; commit.

## Results

`go fix -diff ./...` is silent before and after the hand edits. `mise run
check` green, every package's tests re-run, the rendered manual unchanged.
`mise run lint` green: gofmt, vet, tidy, staticcheck, whitespace, secrets,
large files. `mise run dead` reports only `internal/session`, whose verb is
commented out in `main.go:57` on purpose — the same before as after.

Eight files, +55 −60. Every hand rule with a site is applied:

- `errors.Is(err, io.EOF)` in `fetch.go` — the one Critical rule.
- `slices.Sorted(maps.Keys(m))` at five sites; `sortedFiles` keeps its
  filter, as `slices.DeleteFunc` on the sorted result.
- `cmp.Or` at three sites, each with a fallback that is a constant or a pure
  lookup.

**What the guidelines' CLI adds over a grep.** Its `list` is the checklist
and its `explain` settled every judgement call: `cmp.Or` is wrong when the
fallback runs a subprocess (`keys.go`), and the timeout-cause rule is for
callers that read the cause, which none here do. Each skip above is the
rule's own wording, not a preference.

**The tooling, end to end.** The plugin installs cleanly through the `claude
plugin` CLI, but its skill's first run could not find `go`: the script builds
the CLI from a temp directory under `~/.cache`, and mise shims resolve from
the current directory. A global default, `mise use -g go@1.27.1`, is the
least that works and mise's own suggestion. AGENTS.md now says so, because a
new machine hits the same wall.

## Issues raised

- The plugin is installed at user scope, on this machine. `session.toml` can
  only *block* a plugin (it writes `enabledPlugins: false`); it cannot enable
  one or add a marketplace, so nothing in a repo can give the next
  developer's Claude Code the guidelines. Teaching `internal/session` an
  `enabled_plugins` and `extra_known_marketplaces` is a feature of the tool
  and changes what sync writes for every repo — the Product Owner's call.
- Modern Go holds for every Go repo on the stack, not this one. AGENTS.md's
  own rule says such a thing belongs in the dev skill so fixing it once fixes
  it everywhere. Whether the pointer to the guidelines and "`go fix` first"
  should ride `skill.md` to every repo that pins dev is the Product Owner's
  call; it changes what ships.
