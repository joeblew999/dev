# Stagehand for the browser check — replace the headless-Chrome + node probe with the Go SDK

**File:** `dev/.plans/2026-09-19_0900_stagehand-for-browser-check.md` — refer to this plan by that name.

**Status:** draft, not started · **Created:** 2026-09-19 · **Checked:** 2026-09-19 (against this repo + https://github.com/browserbase/stagehand + https://docs.stagehand.dev/v4/first-steps/quickstart) · **Re-checked:** 2026-09-19 against the module proxy, `go doc` on the real module, and a live `claude -p` run — see [What was verified](#what-was-verified-2026-09-19). The pin moved to v4.1.0 and cost became a first-class constraint.
**Affects:** this repo only (`internal/stage/browser.go`, `internal/stage/stage.go`, `go.mod`/`go.sum`, `mise.toml`, `internal/stage/usage.md` — which gains a browser-check section it does not have today).
**Decisions needed before starting:** the `--bare` trade-off (open question 1). Bare is reproducible, trustworthy and the documented direction of travel, but needs a real `ANTHROPIC_API_KEY`; keyless keeps the subscription login and costs cents a call once the model is pinned, but runs the target repo's hooks and MCP servers on every check. Also whether the old `tools/browser-check.mjs` path stays as fallback or is removed.

## Decision and evidence

`dev check DIR` runs a browser probe when the directory ships `tools/browser-check.mjs` (`internal/stage/stage.go:213-217`). That probe today (`internal/stage/browser.go:71-135`):

1. finds Chrome via `CHROME` or well-known paths (`findChrome`),
2. serves the built server on a free port with `GO_PORT`,
3. launches headless Chrome with `--remote-debugging-port` + throwaway profile,
4. shells `node <probe> <debug-endpoint> <url>`.

That is Playwright-shaped plumbing hand-rolled in Go + a per-app JS script. Stagehand (https://github.com/browserbase/stagehand, MIT, ~24k stars) is the agent browser SDK for exactly this: `act` / `observe` / `extract` over CDP, with TypeScript, Python, and Go SDKs.

**The version the docs give does not exist.** The quickstart says `@v4.0.0`; the proxy says `unknown revision packages/sdk-go/v4.0.0`. The published tags are `v4.0.1 v4.0.2 v4.0.3 v4.1.0`. Pin the latest, confirmed 2026-09-19:

```bash
go get github.com/browserbase/stagehand/packages/sdk-go/v4@v4.1.0
```

The shape, with signatures read from `go doc` on v4.1.0 rather than from the docs site:

```go
browser, _ := stagehand.LaunchLocalBrowser(ctx, &stagehand.LocalBrowserLaunchOptions{})
client, _ := stagehand.Create(ctx, stagehand.CreateOptions{Browser: browser, Model: &model})
page.Goto(ctx, url, nil)
client.Act(ctx, stagehand.ActInstruction("..."), nil)
client.Observe(ctx, &instruction, nil)
stagehand.Extract[T](ctx, client, "...", nil) // → (TypedExtractResult[T], error)

// CreateOptions carries both model paths, and they are mutually exclusive:
//   Model    *ModelConfig    — provider key
//   Generate LLMGenerateFunc — bring-your-own-LLM callback
```

Why it fits here:

| Today | With Stagehand Go |
|---|---|
| `findChrome` + flags + debug port + `waitFor` in `browser.go` | `LaunchLocalBrowser` owns launch; `findChrome`/`CHROME` becomes launch options or goes away |
| per-app `tools/browser-check.mjs` in node + CDP wiring | per-app Go check (or one natural-language instruction) via `act`/`observe`/`extract` |
| `node` required on PATH via mise (`browser.go:81-83`) | no node, no `.mjs`; Go only, which `check` already requires |
| assertions are bespoke JS | `extract` with a typed struct; schema-validated |

One thing that reads as a benefit is a trap here, and it is worth naming before the table is mistaken for all upside. **Self-healing is a liability in a check.** `CreateOptions.SelfHeal` exists to keep automation working when a page changes; a browser check exists to *fail* when a page changes. Those are the same event seen from two sides. Sold as "self-heals when the form redesigns" it sounds like robustness; run as a gate it papers over the exact regressions this is built to catch. So self-heal goes off for checks (step 2), and the proof that it is off is a fixture that must go red (step 5).

Constraints from this repo that shape the plan:

- **Nothing project-specific** (`AGENTS.md`). Stagehand support lands as a stage capability (drive URL, run instruction, extract), not as any one app's check.
- **Secrets come from fnox, never inline** (user memory). Stagehand local browser requires a model + API key passed in code (`OPENAI_API_KEY` in the quickstart; Model Gateway only works with Browserbase cloud — see https://docs.stagehand.dev/v4/configuration/models: "Model Gateway requires Browserbase-hosted browsers. It does not work with local browsers"). So the keyless path is NOT the gateway — it is Stagehand's bring-your-own-LLM callback (`CreateOptions{ Generate }`, docs: "Your callback, bypassing Gateway" / "Your function runs in your process, on your machine, with your own SDKs and credentials"). The callback shells to `claude -p` using the Claude Code subscription OAuth login, so no `ANTHROPIC_API_KEY` is needed. Any provider-key fallback must arrive via `fnox exec --` / env, never as a CLI arg, never committed. Confirmed working on this machine 2026-09-19 with `ANTHROPIC_API_KEY` unset — but see the next two constraints, which are what make this a trade rather than a free win.
- **Every LLM call costs real money, and the default is the expensive one.** Measured 2026-09-19 on this machine, one trivial structured `claude -p` call with no key set:

  | run | model | cost | cache creation / read |
  |---|---|---|---|
  | cold, in-repo | `claude-opus-5[1m]` (inherited) | **$0.4144** | 41,129 / 0 |
  | cold, neutral cwd | `claude-haiku-4-5` | **$0.0677** | 32,917 / 0 |
  | warm, neutral cwd | `claude-haiku-4-5` | **$0.0169**, then **$0.0135** | ~6,000 / 26,101 |
  | warm, in-repo | `claude-haiku-4-5` | **$0.0262** | 10,987 / 22,077 |

  Pinning the model is worth about 30x. The prompt cache survives across separate `claude -p` processes, so only the first call in a window pays full price. Stagehand issues at least one LLM call per `act`/`observe`/`extract`, so a check of 3-10 calls lands around **$0.04-$0.17 warm**, plus roughly $0.07 whenever the cache is cold — which is every CI run. Affordable, but not free, and it scales with how chatty the check is. The callback must pin `--model claude-haiku-4-5` and never inherit the caller's model. Cost belongs in the driver's tests and in `usage.md`, not discovered on a bill.
- **Non-bare `claude -p` runs the target repo's hooks and MCP servers.** `dev check DIR` runs in arbitrary consumer repos, and the headless docs are explicit: "Without `--bare`, a `-p` session runs the hooks in a project's `.claude/settings.json` and connects the servers in its `.mcp.json`, even in a folder you've never trusted." Measurement corrected the cost half of this argument: a neutral working directory still cached 32,917 tokens against the repo's 41,129, so repo context is a few thousand tokens, not the bulk. The reason to control the working directory is **trust and reproducibility, not cost** — running an arbitrary repo's hooks and MCP servers on every check makes the check behave differently in every repo and is squarely against **nothing project-specific**, whatever it costs. However step 2 lands, the callback must run from a controlled working directory, not wherever `check` happens to be.
- **Keep code and usage true.** If flags change (`--path`, new `--browser-*`), `internal/stage/usage.md` changes in the same edit; `go test` holds the skill to the verbs.
- **Commit green before starting; one file at a time, then `go build ./...`; `mise run check`, not `mise run build`** (`AGENTS.md`). The `encoding/json` v1 note does not apply (no JSON migration here).
- **Local runs still need Chrome installed** (quickstart). The "no Chrome → SKIP and pass" behaviour (`errNoChrome`) stays; only the driver changes.

## Plan

### 0. Spike (prove the SDK in this module, no stage changes)

Part of this is already done (2026-09-19, recorded under [What was verified](#what-was-verified-2026-09-19)); the rest stands.

- **A scratch dir outside the repo has no Go toolchain.** `mise ERROR No version is set for shim: go`, and mise offers only 1.26.x globally. Either run `mise use -g go@1.27.1` once as AGENTS.md says, or call the repo's pinned binary by absolute path (`$(mise which go)` from the repo root). The checks below were done the second way.
- ~~`go get ...@v4.0.0`~~ → **done at `@v4.1.0`**: the module resolves, and the API matches what this plan assumes. `go doc` beats the docs site.
- ~~Confirm the keyless `Generate` path~~ — **done**: `claude -p --output-format json --json-schema` with `ANTHROPIC_API_KEY` unset returns `structured_output` from the subscription OAuth login. That is the right field to parse. Normal `claude -p` uses the login from `/login` (Pro/Max/Teams/Enterprise) per https://code.claude.com/docs/en/authentication; `--bare` does not (https://code.claude.com/docs/en/headless).
- **Still to confirm:** `LaunchLocalBrowser` honours the installed Chrome on darwin **and linux**, and `Goto` + `Act` + `Observe` + `Extract[T]` drive a local `GO_PORT` server end to end.
- **Still to confirm:** what a *not-logged-in* `claude` does — SKIP-and-pass like missing Chrome, or a hard fail? The whole SKIP contract in step 2 rests on this being cheaply detectable, ideally without spending a call.
- ~~Still to measure: cost with the model pinned~~ — **done**, four runs recorded in the cost constraint above: $0.0135-$0.0677 a call on `claude-haiku-4-5` against $0.4144 unpinned, and the prompt cache survives across processes. **Still to measure:** one *real* check end to end once a fixture exists. That number, not this one, decides gate-vs-report (open question 4).
- Confirm the provider-key fallback only if keyless is rejected on cost: `fnox exec -- go run ./scratch` with the key in fnox config; record which key name here before step 2.
- Delete the scratch; nothing spikes into `internal/stage`.

### 1. Dependency + toolchain

- Add the SDK to `go.mod` with an exact pin (`v4.1.0`, not `latest` — reproducibility over auto-patching per user memory).
- **Accept the dependency tail deliberately.** This repo has two direct dependencies and one indirect today. The SDK brings **21 modules**: otel x4, testify, invopop/jsonschema, coder/websocket, two yamls, x/tools, x/mod and the rest. For a tool AGENTS.md keeps deliberately bounded, that is the largest cost of this change after the LLM bill. If it is not worth it, the answer is to keep `probe` and stop here.
- `mise run check` (build + `go test ./...` + snapshot release). No new tool in `mise.toml [tools]` expected (Go only); if the SDK needs node for any step, stop and re-plan — the point is to drop node.

### 2. `internal/stage`: Stagehand driver beside `probe`

- New file (e.g. `internal/stage/stagehand.go`): `stagehandCheck(out, server, path, instruction/extract-spec)` mirroring `probe`'s contract — serve app on free port, `waitFor` URL, launch browser, run, close, return error. Keep `freePort`/`waitFor`/`stop` reuse.
- Model wiring: default is `CreateOptions{ Browser, Generate: claudeCodeGenerate }` (keyless, local browser). `claudeCodeGenerate` maps `LLMGenerateParams.AsStructured()` (messages, systemPrompt, ResponseFormat name+schema) onto `claude -p --output-format json --json-schema '<schema>'`, parses `.structured_output` (confirmed present and correct on a real run), and returns `StructuredGenerateResult`. Requires `claude` CLI on PATH plus a prior `/login`; missing CLI or expired login must surface the same SKIP-and-pass contract as missing Chrome (never fail `check` for missing local auth). Provider-key `ModelConfig` is fallback only.
- **Pin the model; separately, control the working directory.** The callback passes `--model claude-haiku-4-5` explicitly and never inherits the caller's model — measured at about 30x. It also runs from a working directory that is *not* the repo under check, so it does not load that repo's `CLAUDE.md`, hooks, plugins and MCP servers; that second one buys trust and reproducibility rather than money. If open question 1 lands on `--bare`, both fall out for free and the cost of a key comes back instead. Either way a test must assert the model flag is passed: nothing else will catch a silent regression from cents back to dollars.
- Keep `probe` untouched until the new driver is green; one file at a time, `go build ./...` after each.
- Unit tests where there are seams: keep `TestFindChrome`-style coverage for whatever replaces launch config (env override, missing browser → SKIP, not fail).
- **`SelfHeal: false`, explicitly, with a comment saying why.** The default exists to keep automation alive across a redesign; a check wants the opposite. Left on, it makes the driver robust and useless at once — it will route around a broken picker and report green. The same goes for retries: a check that retries until it passes is not a check.
- Decide here: `tools/browser-check.mjs` (node script) vs `tools/browser-check.go` (or inline instruction + extract schema). Recommendation: new convention `tools/browser-check.go` (a `go run` target) with `browser-check.mjs` honoured as deprecated fallback for one release, then removed.

### 3. Wire into `Check`

- `stage.go: Check` prefers the Stagehand path when present, falls back to `probe` while `.mjs` exists (or errors per the step-2 decision). No behaviour change for dirs with neither.
- **A way to refuse to skip.** The SKIP-and-pass contract has three triggers — no Chrome, no `claude`, logged out — and each one quietly turns a browser check into a pass. On a developer's laptop that is right. In CI it means a misconfigured runner passes every browser check forever and nobody finds out. So `check` needs a mode (`--require-browser`, or `DEV_REQUIRE_BROWSER=1` — whichever fits the verb shape) that turns every SKIP into a failure, and CI runs that way. Whatever it is called it is a flag change, so `internal/stage/usage.md` changes in the same edit.
- `CHROME`/`NodeBin` docs: `CHROME` stays if it maps to launch options; the "node is not on PATH" error goes when `.mjs` goes.
- Update `internal/stage/usage.md` + `cli/*.md` prose in the same edits; `mise run skill` regenerates; `go test` holds it.

### 4. Secrets + tasks

- Default path needs NO secret: `claude` subscription login (Keychain on macOS / `~/.claude/.credentials.json` on Linux, mode 0600) — nothing in fnox, nothing as a `dev check` flag (flags leak via `ps`; secrets are never arguments).
- CI/unattended where browser login isn't available: `claude setup-token` mints a one-year OAuth token (the subcommand exists in v2.1.278), surfaced as `CLAUDE_CODE_OAUTH_TOKEN` via `fnox exec --` (still no API key; subscription-billed).
- **Tested 2026-09-19: bare mode does NOT read `CLAUDE_CODE_OAUTH_TOKEN`.** `claude --bare -p` with no key prints `Not logged in · Please run /login`, and setting a bogus `CLAUDE_CODE_OAUTH_TOKEN` produces the *identical* message — were the variable read, a malformed token would fail differently. The plan's original claim was right, and the escape hatch is closed: bare needs a real `ANTHROPIC_API_KEY`. `CLAUDE_CODE_OAUTH_TOKEN` is therefore only useful on the non-bare path.
- Provider-key fallback (if keyless is ever skipped): via fnox only (`fnox exec --`), surfaced as env (e.g. `OPENAI_API_KEY`), never a flag.
- If a consumer repo needs the CI token/key, that is a `mise.toml [tasks]` + fnox entry in that repo, not here. This repo's own `mise.toml` gains nothing except possibly a documented env passthrough for its own fixture app.
- Record here which path open question 1 settled on, with the step-0 numbers behind it: bare or keyless, the token name for CI, and the provider key name only if a fallback is kept.

### 5. Fixture + proof

A green check is not evidence. For a probe that either found a DOM node or did not, green meant something. For a check an LLM judges, green means nothing until red has been demonstrated on demand. This step builds that demonstration, and it is the step that decides whether any of the rest is trustworthy.

- **Build a fixture; there is nothing to reuse.** No `tools/browser-check.mjs` exists anywhere in this repo, and `probe` has no test coverage — only `findChrome` is tested (`internal/stage/browser_test.go`). This plan replaces a path that has never been proven end to end, so this step is a build, not a swap. Budget for it accordingly.
- The fixture needs genuine client-side behaviour (the picker-that-swaps-models case from `browser.go:1-3` is the bar — curl cannot check it).
- **Build a broken twin, and have `go test` run both.** The working fixture must go green; a deliberately broken copy — the picker wired to the wrong handler, say — must go **red**. A check never observed failing is not a check, it is a green light with a browser attached. This is the highest-value item in the plan and the whole reason step 5 exists.
- **Assert it actually ran.** Under `--require-browser` (step 3) every SKIP is a failure, and CI runs that way. `total_cost_usd` from the callback doubles as a liveness signal: $0 means no LLM call happened, whatever the check printed.
- **Measure the flake rate before calling it a gate.** Run both fixtures N times and record the pass rate *here, in this plan*. N/N on both, or it is a report and not a gate — the same distinction this repo already draws for `mise run dead`. Non-determinism is fine in a report and fatal in a gate.
- **The model version is part of the gate.** The pinned model is what decides pass or fail, so bumping it re-runs this whole evidence set. Record which model the numbers were taken with.
- `mise run check` green with Chrome present; SKIP-and-pass with Chrome absent (same contract as today); SKIP-and-pass with `claude` absent or logged out; and all three of those SKIPs failing under `--require-browser`.
- `mise run lint` (hk: gofmt, vet, tidy, staticcheck, whitespace, secrets) clean; `mise run dead` reviewed, not gated.

### 6. Docs

- `internal/stage/usage.md` gains a browser-check section **written from scratch** — today that file is 385 bytes about stages in general and never mentions the browser check, Chrome, node or the SKIP contract. Write: what triggers it, what it needs (Chrome, plus `claude` logged in or a key via fnox), the SKIP contract, and what a run costs.
- Skill regenerates from verbs; no hand-kept copies.
- Move this plan to `.plans/done/` on completion per `.plans/README.md`.

## What was verified (2026-09-19)

Checked against the module proxy, `go doc` on the real module, the live docs, and a real `claude -p` run on this machine.

**Held up:**

- All four line references are exact: `stage.go:213-217`, `browser.go:71-135`, `browser.go:81-83`, `browser.go:1-3`.
- `CreateOptions.Generate LLMGenerateFunc` is real, alongside `Model *ModelConfig`.
- `LaunchLocalBrowser(ctx, *LocalBrowserLaunchOptions) (*Browser, error)` is real.
- `Extract[T any](ctx, *Stagehand, string, *StagehandClientExtractOptions) (TypedExtractResult[T], error)` is real, and this plan's call shape matches it.
- "Model Gateway requires Browserbase-hosted browsers. It does not work with local browsers" — verbatim and current. The gateway really is not the keyless path.
- `claude -p` keyless works: run with `ANTHROPIC_API_KEY` unset, it returned a clean result from the subscription login.
- `structured_output` is the right field to parse — confirmed on a real run and in the headless docs.
- `--bare` "never reads OAuth credentials or the system keychain" and needs `ANTHROPIC_API_KEY` — verbatim and correct.
- `--json-schema`, `--output-format` and `setup-token` all exist in `claude` v2.1.278.
- "bare mode does NOT read `CLAUDE_CODE_OAUTH_TOKEN` either" — flagged unverified in the first pass, then tested. The plan was right.

**Did not hold up:**

- `v4.0.0` does not exist, though the docs site says it does. Pin `v4.1.0`.
- Step 0's scratch dir has no Go toolchain: there is no global `go`.
- Cost was never considered — $0.41 for one trivial call.
- Non-bare `-p` running the target repo's hooks and MCP servers was never considered.
- Step 5's fixture does not exist and `probe` has no test coverage, so this is a build, not a swap.
- The 21-module dependency tail was never counted.
- Self-healing was listed as a benefit. `CreateOptions.SelfHeal` is real, but in a *check* it is a liability — see the note under the comparison table.
- There was no answer to "how would we know it works". Green was the only proposed proof, and for an LLM-judged check green proves nothing by itself. Step 5 now carries that burden.

One correction to the first review, which was measured rather than reasoned: it claimed the 41k cached tokens were mostly the repo's `CLAUDE.md` and hooks, and that controlling the working directory would cut the bill. It does not — a neutral directory still caches ~33k. Controlling the working directory is a trust argument, and the cost argument belongs entirely to pinning the model.

## Open questions (for the Product Owner)

1. **`--bare` or not. This is the real decision, and it is a trade rather than a preference.**

   | | keyless (`claude -p`) | bare (`claude -p --bare`) |
   |---|---|---|
   | Credential | subscription OAuth login, no key | needs a real `ANTHROPIC_API_KEY`; `CLAUDE_CODE_OAUTH_TOKEN` is **not** read by bare (tested, step 4) |
   | Cost | $0.0135-$0.0677 a call with haiku pinned, ~$0.41 without | cheaper again, but unmeasured here — no key to test with |
   | Reproducibility | loads the target repo's `CLAUDE.md`, hooks, plugins, MCP | docs: "the same result on every machine" |
   | Trust | runs an arbitrary repo's hooks and MCP servers | reads none of them |
   | Direction of travel | — | docs: "recommended mode for scripted and SDK calls, and will become the default for `-p` in a future release" |

   Pinning the model brings the keyless path to cents a call, so **cost no longer decides this**. And the `CLAUDE_CODE_OAUTH_TOKEN` escape hatch turned out to be closed (step 4), so the trade is real and unavoidable: keyless costs nothing to set up but runs an arbitrary repo's hooks and MCP servers on every check; bare is clean, reproducible and the documented direction of travel, but `dev` would carry one `ANTHROPIC_API_KEY` in fnox. Since this tool exists to run in many repos it does not control, the trust argument reads stronger than the no-key argument — but that is the Product Owner's call, not the plan's.

   Browserbase + Model Gateway is not the keyless answer either way — the gateway needs `BROWSERBASE_API_KEY` plus a cloud browser, so it stays a follow-up, not this plan.

2. Remove `browser-check.mjs` outright or deprecate-then-remove? Recommendation above is deprecate-then-remove.
3. Cloud runs in scope? This plan is local-browser only. Browserbase cloud (`browserbase.launch`, caching, recordings) is a follow-up plan if wanted.
4. **Gate or report?** If step 5's flake measurement comes back N/N on both fixtures, this can fail a build. If it does not, the honest options are to keep it as a report (print findings, never fail — the `mise run dead` precedent), or to keep `probe` for the deterministic assertions and use Stagehand only for what a DOM query genuinely cannot reach. Decide once the number exists, not before.

## Non-goals

- No `dev deploy`/`url`/`logs` changes, and no new verb. This was going to be `check` internals only, but step 3's `--require-browser` is a flag on an existing verb, so `cli/` is touched after all — one flag, not a new command shape, and `internal/stage/usage.md` changes in the same edit. Load the `cli` skill before that edit.
- No Browserbase cloud dependency, no committed keys, no `npx` (never run bare `npx`; blocks on install prompt).
- No `encoding/json` migration; no modern-go refactors beyond what `go fix` already owns.
