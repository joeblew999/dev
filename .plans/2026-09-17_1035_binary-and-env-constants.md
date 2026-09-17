# Binaries and env names become constants

**File:** `dev/.plans/2026-09-17_1035_binary-and-env-constants.md` — refer to this plan by that name.

**Status:** done 2026-09-17 — all 9 steps green, full suite + skill check pass ·
**Created:** 2026-09-17 10:35 · **Finished:** 2026-09-17
**Affects:** this repo only — named constants for external binary names and
environment variable names, used at every `exec` call site and `Getenv`.
No behaviour, no task, no generated skill, no release artifact, no other repo.
**Decisions needed before starting:** none.

## Decision and evidence

Binary names (`"wrangler"`, `"flyctl"`, `"fnox"`, `"go"`, `"git"`, `"node"`,
`"npm"`, `"tinygo"`, `"gh"`, `"claude"`, `"mise"`, `"ps"`, `"lsof"`) and env
names (`"FLY_ORG"`, `"GOOS"`, `"GOARCH"`, `"CHROME"`, `"GO_PORT"`) are string
literals at every call site. A rename or typo in one site compiles and fails
at runtime, in CI or on a developer's machine. Constants make the full set
visible in one place per package and let the compiler catch drift.

Rule: a constant lives in the package that owns the tool. `cloudflare`
owns `wrangler`; `fly` owns `flyctl`; `fnox` owns `fnox`; `stage` owns the
build tools it drives (`go`, `npm`, `node`, `tinygo`, `gsx`); `session`
owns `claude`, `git`, `ps`, `lsof`; `scaffold` owns `mise`; `secrets`
owns `gh`. `GOOS`/`GOARCH`/`GO_PORT`/`CHROME` live where they are read.
`FLY_ORG` lives in `fly` beside `ConfigFile`.

Out of scope: flags and subcommand args (`"dev"`, `"--env"`, `"exec"`,
`"--"`), which are call-specific, not names; usage prose, which documents
rather than invokes; test fixtures.

## DOD

- [x] Every `exec.Command`/`exec.LookPath`/`execIn`/`run` binary name in
  non-test code is a named constant owned by the package that owns the tool.
- [x] `os.Getenv`/`os.Setenv` names in non-test code are named constants.
- [x] `go vet ./...` clean, `go test ./...` green, `gofmt` clean.

## Work

- [x] 1. `cloudflare`: `WranglerBin = "wrangler"`, `FnoxBin = "fnox"`;
  used in `logs.go`, `smoke.go`, `deploy.go`. Done 2026-09-17.
- [x] 2. `fly`: `FlyctlBin = "flyctl"`, `FnoxBin = "fnox"`, `OrgEnv = "FLY_ORG"`;
  used in `fly.go` (`exec`, `logs`, `Getenv`). Done 2026-09-17
  (test's `t.Setenv("FLY_ORG", …)` keeps the literal — fixture, out of scope).
- [x] 3. `fnox`: `Bin = "fnox"`; used in `Get`, `Set`, `Exec`. Done 2026-09-17.
- [x] 4. `stage`: `GoBin`, `NpmBin`, `NodeBin`, `TinyGoBin`, `GsxTool`,
  `WranglerBin`, `FnoxBin`, `GoOSEnv`, `GoArchEnv`, `GoPortEnv`, `ChromeEnv`;
  used in `stage.go`, `browser.go`. Done 2026-09-17. Mode tags (`"go"`/
  `"tinygo"` as strings) and `gsx` subcommand args stay literal.
  `browser_test.go` fake getenv uses `ChromeEnv`.
- [x] 5. `session`: `ClaudeBin`, `GitBin`, `PsBin`, `LsofBin` in `session.go`;
  used in `mcp.go`, `verify.go`, `sessions.go`, `bump.go`. Done 2026-09-17.
  Process-basename match (`"claude"`, `"claude.exe"` in `sessions.go:94`)
  stays literal — name matching, not invocation.
- [x] 6. `scaffold`: `GoBin`, `MiseBin`; used in `tidy`, `generateSkill`,
  `latest`. Done 2026-09-17.
- [x] 7. `secrets`: `GhBin = "gh"`; used in `CI`. Done 2026-09-17.
- [x] 8. `release`: `GoreleaserBin`, `PackslipBin`, `GhBin`; used at all
  `run`/`out` call sites. Done 2026-09-17.
- [x] 9. `mise run test` green; recorded here. Done 2026-09-17:
  `go build ./...`, `go vet ./...` clean, `go test ./...` all green,
  `gofmt` clean, `.bin/dev skill --check` all three copies up to date.

## Not in this plan

- File-name constants (`wrangler.toml`, `fly.toml`, `mise.toml`, `go.mod`) —
  done separately (`cloudflare.ConfigFile`, `fly.ConfigFile` already;
  `mise`/`go.mod` left as literals, different meanings per package).
- Skill-dir constants — done (`cli.ShippedDir`, `ClaudeDir`, `AgentsDir`,
  `SkillFile`).
- The `internal/` move (`2026-09-17_1022_internal-is-the-default.md`):
  land that first or rebase these consts onto `internal/<pkg>` paths.
