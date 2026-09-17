# AGENTS.md rules propagate through the dev skill

**File:** `dev/.plans/2026-09-17_1055_agents-rules-ride-the-skill.md` — refer to this plan by that name.

**Status:** draft, not started · **Created:** 2026-09-17 10:55
**Affects:** this repo only — `main.go` `skillHead`/`skillTail` gain the
repo-wide agent rules; `dev skill` regenerates; release ships. No new
mechanism: `release/` already ships every `skills/<name>/`, mise already
links the pinned skill into consumer repos.
**Decisions needed before starting:** which rules are repo-wide (all repos
on the stack) vs dev-specific (this repo only). Default: everything under
`AGENTS.md` "Skills" and "Working rules"-shaped lines is repo-wide; Layout
and tool-guts stay dev-specific.

## Decision and evidence

`AGENTS.md` in this repo says what an agent working here must follow. Other
repos on the stack need the same rules, but there is no mechanism that
carries them: `dev init` writes a one-shot `AGENTS.md.tmpl` copy, which
drifts from the day it is written.

The mechanism already exists and is proven: `main.go` `skillHead`/`skillTail`
render into `skills/dev/SKILL.md`, `dev release` ships every `skills/<name>/`
as a packslip resource (`release.go:194-197`), and consumer repos pin dev in
`mise.toml`, which links the skill into `.claude/skills/dev` and
`.agents/skills/dev`. A rule written once in `skillHead`/`skillTail` reaches
every repo that bumps its dev pin — no new plumbing, no new task, no new
file.

So: repo-wide rules live in `skillHead`/`skillTail`, not in `AGENTS.md`.
`AGENTS.md` keeps only what is dev-specific (Layout table, tool-guts rules).
The scaffold's `AGENTS.md.tmpl` keeps only what is repo-specific (repo name,
its own layout) and points at the dev skill for the rest — which it already
does ("The stack's rules live in the dev skill").

## DOD

- [ ] `main.go` `skillHead`/`skillTail` contain every repo-wide agent rule
  currently in `AGENTS.md`.
- [ ] `AGENTS.md` contains only dev-specific rules; nothing repo-wide remains.
- [ ] `dev skill` regenerated (three copies), `go test ./...` green.
- [ ] Next `dev release` ships the updated skill; consumer repos get it on
  pin bump.

## Work

- [ ] 1. Sort `AGENTS.md` line by line into repo-wide vs dev-specific.
  Repo-wide candidates: mise-task rule, nothing-project-specific, skill-is-
  generated, one-thing-per-package, test-for-real, commit-only-named-files,
  tone, plans/DOD/issues discipline, comments-why. Dev-specific: Layout
  table, `internal/` boundary, `cli` contract notes.
- [ ] 2. Move repo-wide rules into `skillHead`/`skillTail` in `main.go`,
  phrased for any repo on the stack (no `dev`-specific paths).
- [ ] 3. Trim `AGENTS.md` to dev-specific content; each removed rule replaced
  by a pointer to the dev skill.
- [ ] 4. Check `AGENTS.md.tmpl` needs no change (it already defers to the
  skill); adjust only if it duplicates a moved rule.
- [ ] 5. `dev skill`, `go test ./...`, record results here.

## Not in this plan

- Versioning or pinning per rule — the skill ships whole with the dev
  release; repos opt in by bumping the pin.
- Enforcing rules in consumer repos — the skill informs; `hk` and
  `session:verify` enforce.
- The `stylegen` port and release (skill-for-every-repo steps 6–7) —
  independent; either order works.
