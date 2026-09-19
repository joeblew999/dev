# Google Search conformance checks — Go tool shortlist

**File:** `dev/.plans/2026-09-19_1200_google-search-conformance-tools.md` — refer to this plan by that name.

**Status:** BUILT — `dev seo` ships write, validate and check; see `internal/seo/catalogue.md` for the inventory and `internal/seo/usage.md` for the manual · **Created:** 2026-09-19 · **Checked:** 2026-09-19 (against developers.google.com/search + web.dev/vitals) · **Recency:** 2026-09-19 (`go list -m` timestamps + `gh repo view` pushedAt) · **Spiked:** 2026-09-19 (all 15 candidates pinned, built under `go 1.27.1`, live-tested on `example.com`; see Spike) · **Reviewed:** 2026-09-19 (the tools — 10 findings, see Review 1) · **Re-reviewed:** 2026-09-19 (the fit to this repo — 10 findings, four of them blocking, see Review 2; all folded in below)

**Affects:** this repo only if accepted — `internal/seo/` (new, with its `usage.md`), `main.go` verb table, `mise.toml` (`[tools]` pins + a post-deploy task sketch), `skills/dev/SKILL.md` via `dev skill`. No new entries in this repo's `go.mod`: every tool is a shelled binary pinned in `mise.toml`, and the JSON-LD step is own code. No change to `cli/` — see the exit-code decision. `internal/seo/usage.md` needs no `mise.toml` sources edit either: the build glob `internal/*/usage.md` (`mise.toml:50`) already covers it.

**Decisions (were open, now answered — the first three were wrong in the first draft and are corrected here):**

- **Shelled binaries, found the way this repo finds every other tool.** A `ScoutlyBin`/`MuffetBin` const + `exec.LookPath` + an error naming the pin to add, as `internal/stage/browser.go:81-83` does for node and `internal/cloudflare/cloudflare.go:25` for wrangler. *Not* `stage.run()`: it is unexported and sets `cmd.Stdout = os.Stderr` on purpose (`internal/stage/stage.go:51-62`), so no JSON can be parsed through it.
- **`dev seo URL`, run after deploy, in a task of its own.** Not in `validate`: `validate` is what `deploy` runs *first* (`skill.md:36`), so it has no live URL — on a first deploy there is nothing to crawl, and later it would grade the previous deployment. Not in `check` either, for the same reason. A consumer repo wires `<cmd>:seo` as a post-deploy task, chained the way `wait` is.
- **Exit 0 pass / 1 fail.** Not 0/1/2: `cli` spends 2 on usage errors and gives a verb no way to choose a code (`cli/command.go:66`, `cli/command.go:141-146`). Gate-vs-crash is said in the message and in the JSON (`"outcome": "gate"|"error"`), not in the exit status. Widening `cli` to carry codes would change the public API every repo on the stack builds on — out of scope here; its own plan if ever wanted.
- **Offline-only v1**; online (CrUX + lab PSI) is v2, behind `fnox` env, skipped without creds.
- **JSON-LD v1 is own code, zero deps**: extract, parse, check required properties. No `json-gold`/`jsonschema` import — there is no published `schema.org` JSON Schema to validate against, so real validation is a later decision with its own evidence (Review 2, finding 5).
- **No `node lighthouse` fallback** — Go + hosted APIs only.

## What Google requires (2026)

1. **Technical requirements** (`search/docs/essentials/technical`): Googlebot not blocked, `HTTP 200`, indexable file type, no spam.
2. **Crawling** (`crawling/docs/`): `robots.txt` RFC 9309 at `/robots.txt`, UTF-8, `<=500 KiB`, `Allow/Disallow/Sitemap/User-agent` only, `*` + `$`, longest-match wins, `4xx`=ignore / `5xx`=pause; `HTTP/1.1`+`HTTP/2`, `gzip, deflate, br`, first `15MB` only, `ETag`/`Last-Modified` caching; smartphone Googlebot only (mobile-first); evergreen Chromium renders JS so links must be `<a href>`.
3. **Sitemaps** (`sitemaps.org`): `50k URLs / 50MB` per file, `sitemapindex` for large sites, `lastmod`, video/image/news extensions, advertised as `Sitemap: <absoluteURL>` in `robots.txt`.
4. **Page experience + Core Web Vitals** (`search/docs/appearance/page-experience`, `core-web-vitals`): `LCP <=2.5s`, `INP <=200ms`, `CLS <0.1` at 75th percentile mobile+desktop; `HTTPS`; no intrusive interstitials. Field truth is CrUX; lab proxy is Lighthouse (`TBT` proxies `INP`).
5. **Structured data** (`search/docs/appearance/structured-data`): Rich Results Test for Google-specific, Schema Markup Validator (`validator.schema.org`) for generic `schema.org` JSON-LD/Microdata/RDFa.

## Decision table (the one table that decides; everything below is reference)

One crawl owner; binaries shelled (found by `exec.LookPath`, never imported), overlaps kept where no tool covers the layer. Names, not W-numbers, so insertion never drifts.

| Layer | Google need | Tool + pin | How `dev` uses it | Why this one |
|---|---|---|---|---|
| Crawl + on-page | crawlability, titles, meta, H1, canonical, images, OG, robots 512 KiB, sitemap 50 MiB/50k | [`scoutly v0.5.0`](https://github.com/nelsonlaidev/scoutly) ([pkg audit](https://pkg.go.dev/github.com/nelsonlaidev/scoutly/audit)) | Shell `scoutly <url> --format json` through `internal/seo`'s own runner — found by `exec.LookPath`, stdout captured, a missing binary erroring with the pin line to add. Not `stage.run()`: unexported, and it streams stdout to stderr (`internal/stage/stage.go:51-62`). Pin in `mise.toml [tools]` as `"go:github.com/nelsonlaidev/scoutly/cmd/scoutly" = "v0.5.0"`. | Best audit core, live-verified. Shelled (not imported) because the `audit` lib drags the charm TUI tail (bubbletea/huh/lipgloss/kong — inside the measured 69-module scratch tail) into `dev`'s 2-dep module. |
| Link layer | broken/redirected links, sitemap-as-root, throttling | [`muffet v2.11.5`](https://github.com/raviqqe/muffet) ([docs](https://raviqqe.github.io/muffet/)) | Shell `muffet --follow-robots-txt <url>` through the same runner; JSON output parsed, not streamed. Pin as `"go:github.com/raviqqe/muffet/v2" = "v2.11.5"` (note `/v2` suffix). | 2.6k stars, 67 releases, live-verified 4xx handling. Own crawler overlaps scoutly's — kept because scoutly's link reporting is summary-level while muffet is the thorough checker; both run, results merged. Dep licenses checked: `fasthttp` MIT, `go-flags` BSD-style, `aurora` public domain, `gopher-parse-sitemap` MIT, `ratelimit` MIT, `rootcerts` BSD-2. |
| Gate | score, baseline regression, exit contract | [`Erose112/seo-audit`](https://github.com/Erose112/seo-audit) @ `2b1e6fc29596` (pseudo-version, no tag — repin at implementation) | Copy the gate shape, not the binary: `--fail-below`, `--baseline` + `compare`, stdout-pure JSON, strict rules for template checks. Its **exit codes are not copied** — `cli` owns 2 for usage errors and lets no verb pick a code (`cli/command.go:66`, `141-146`), so `dev seo` exits 0 pass / 1 fail and says gate-vs-error in the JSON. MIT: the notice travels with any copied lines. | Best CI-gate design, live-verified (85/100 on example.com). Too narrow (9 checks) to run as the checker. |
| Field CWV | p75 LCP/INP/CLS + FCP/TTFB, official thresholds | [`cruxpeek`](https://github.com/printemps-tokyo/cruxpeek) @ `fa6ae369c9eb` (pseudo-version, no tag — repin at implementation) | Copy the stdlib-only client pattern into `internal/seo` (no new dep); MIT, so the notice travels with any copied lines. Key only from `CRUX_API_KEY` via `fnox exec --`, never a flag. Its own 0/1/2/3 exit codes are not copied — see the Gate row. | Stdlib-only, exit 0/1/2/3 contract, offline fixtures. Pulling `google-api-go-client` (weekly releases, large tail) for CrUX alone is not worth it. |
| JSON-LD | schema.org checks beyond presence | own code in `internal/seo` — **no new deps in v1**; [`json-gold v0.8.0`](https://github.com/piprate/json-gold) + [`jsonschema/v6 v6.0.3`](https://github.com/santhosh-tekuri/jsonschema) (both Apache-2.0) recorded but deferred | Extract `script[type=application/ld+json]` from the crawled pages, parse, check required properties per type. Zero imports. | Whole tools only detect JSON-LD presence. Real validation needs a `schema.org` JSON Schema, and schema.org publishes none — hand-writing and maintaining one here is its own decision, not a step in this plan (Review 2, finding 5). Importing the two libs would take `dev` from 2 direct deps to 4, plus the `ld` tail the Spike measured (`cayleygraph/quad`, `pquerna/cachecontrol`). |
| Lab PSI | lab Lighthouse + field in one call | [`pagespeedonline/v5`](https://pkg.go.dev/google.golang.org/api/pagespeedonline/v5) @ `v0.298.0` (repin at implementation) | Own-code call in `internal/seo` v2 only, key via `fnox`, skipped without creds. | Official client; kept to lab data only since CrUX comes via the cruxpeek pattern. |
| Reference only | check taxonomy (`robots-test`, `verify-bot`, sitemap coverage, mobile-vs-desktop, hreflang) | [`micelio`](https://github.com/jlhernando/micelio-crawler) (needs node+SQLite — not shelled), [`seo-automation`](https://github.com/Piyush8296/seo-automation) (175+ checks / 22 categories, stale 5 mo), [`seonaut`](https://github.com/StJudeWasHere/seonaut) (787 stars, server+DB) | Borrow checklist items into `internal/seo` fixtures; never depend, never shell. | Taxonomy sources, wrong shapes (node/frontend or server+DB). |
| Watch | re-check after milestones | [`CrawlGrade`](https://github.com/pan-dolina/CrawlGrade) (Apache-2.0, cobra+x/net only, pre-release) | Nothing now; re-check after first release. | Smallest tail, clean scope, single author, no releases yet. |
| Dropped | — | `colly/v2` (own crawler redundant with scoutly), `snabb/sitemap` (scoutly covers index+gzip+50k caps), `temoto/robotstxt` direct (both binaries bundle it; own-code only needs status-code rules), `brokli` (pick one link layer: muffet wins on 67 releases vs 4), `gopherseo` (narrow; Colly wiring reference at most), `go-seo-analyser` (no LICENSE file found; Rod/Chrome breaks pure-CLI), `deadsniper` (minimal; only if muffet rejected), `seo-auditor`/`Audit-Ant` (DB/node shapes) | — | Recorded so the next reader does not re-spike them. |

Single-purpose lib table (#1–#8) and recency table below are now reference for the above, not competing decisions. `temoto/robotstxt` "most-used" claim softened: it is the REP parser bundled by muffet, scoutly (via its own fetcher), gopherseo, Erose112 and go-seo-analyser — ubiquity observed in this spike, not download counts cited.

## Tool list (reference for the decision table)

| # | Google need | Go tool | Why it fits | Notes |
|---|---|---|---|---|
| 1 | `robots.txt` parse + `Test(path, Googlebot)` | [`github.com/temoto/robotstxt`](https://github.com/temoto/robotstxt) ([pkg](https://pkg.go.dev/github.com/temoto/robotstxt)) | Most-used Go REP parser, `Allow/Disallow` + `* $`, group precedence | Pin exact; reject `jimsmart/robotstxt` (stale). Size `>500KiB` warn, non-UTF-8 warn, `crawl-delay` ignore-warn in own code. |
| 2 | `sitemap.xml` / index / gzip parse | [`github.com/snabb/sitemap`](https://github.com/snabb/sitemap) ([pkg](https://pkg.go.dev/github.com/snabb/sitemap)) | Parses sitemap + index, handles gzip, low deps | Generate side (if needed): own-code (dropped [`ikeikeikeike/go-sitemap-generator`](https://github.com/ikeikeikeike/go-sitemap-generator), stale 2019). Own code enforces `50k/50MB`, `lastmod` shape, absolute URLs. |
| 3 | HTML audit: `title/meta/canonical/hreflang/h1/alt/OG/viewport/favicon`, link graph, status chain | [`golang.org/x/net/html`](https://pkg.go.dev/golang.org/x/net/html) + [`github.com/PuerkitoBio/goquery`](https://github.com/PuerkitoBio/goquery) ([pkg](https://pkg.go.dev/github.com/PuerkitoBio/goquery)) | `x/net/html` is stdlib-adjacent parser; `goquery` is jQuery-shape queries, maintained | Crawl internal links with [`github.com/gocolly/colly/v2`](https://github.com/gocolly/colly) ([pkg](https://pkg.go.dev/github.com/gocolly/colly/v2)) (bounded, same-host, respects robots via #1). No regex scraping. |
| 4 | JSON-LD extract + validate | [`github.com/piprate/json-gold`](https://github.com/piprate/json-gold) ([pkg](https://pkg.go.dev/github.com/piprate/json-gold)) + [`github.com/santhosh-tekuri/jsonschema/v6`](https://github.com/santhosh-tekuri/jsonschema) ([pkg](https://pkg.go.dev/github.com/santhosh-tekuri/jsonschema/v6)) | `json-gold` is JSON-LD expand/flatten; `santhosh-tekuri` is maintained draft 2020-12 validator | **Superseded for v1** (see Decisions): no import, and no vendored shapes — schema.org publishes no JSON Schema, so v1 extracts, parses and checks required properties in own code. Kept here for the day validation is decided on its own: expand via the `ld` subpackage, and use `v6` (`v6.0.3`, 2026-06-28), not `v5` (`v5.3.1`, 2023-07-22). Microdata/RDFa later still. |
| 5 | Lab + field performance, official | [`google.golang.org/api/pagespeedonline/v5`](https://pkg.go.dev/google.golang.org/api/pagespeedonline/v5) ([repo](https://github.com/googleapis/google-api-go-client)) | Official PageSpeed Insights v5 client: lab Lighthouse + field CrUX in one JSON | Key via `fnox exec --`, never arg. CrUX-only path: direct `net/http` to `chromeuxreport.googleapis.com/v1/records:queryRecord`. Part of `google.golang.org/api v0.298.0` (2026-09-14), weekly releases. |
| 6 | Index / sitemap / CWV report state | [`google.golang.org/api/searchconsole/v1`](https://pkg.go.dev/google.golang.org/api/searchconsole/v1) ([repo](https://github.com/googleapis/google-api-go-client)) | Official: `urlInspection.index`, `sitemaps.*`, CWV report surface | OAuth, not API key; `fnox` only. Offline verb must pass without it. Same module as #5, `v0.298.0` (2026-09-14). |
| 7 | HTTP semantics: redirects, `X-Robots-Tag`, `ETag`, `HSTS`, `Brotli`, `>15MB` warn | stdlib [`net/http`](https://pkg.go.dev/net/http) + [`crypto/tls`](https://pkg.go.dev/crypto/tls) | No dep needed; Googlebot UA string + mobile UA both sent | Check `200`, redirect hops `<=5`, `noindex/nofollow` in meta + header, `canonical` absolute, `hreflang` reciprocity (phase 2). [`x/net v0.59.0`](https://pkg.go.dev/golang.org/x/net) (2026-09-08, [repo](https://github.com/golang/net)) if `x/net/html` needed for #3. |
| 8 | Lab when no key (fallback) | existing [`node lighthouse`](https://github.com/GoogleChrome/lighthouse) / `tools/browser-check.mjs` via `CHROME` | `stage.probe()` pattern already shells node; Lighthouse has no Go port | Pin via `mise.toml [tools]`; never bare `npx`. Long-term: [`chromedp`](https://github.com/chromedp/chromedp) lab harness only if keyless gate demanded. **Dropped** — see Decisions. The in-`check` browser probe and the fate of `tools/browser-check.mjs` belong to `.plans/2026-09-19_0900_stagehand-for-browser-check.md`, not here. |

Not selected as libs: hand-rolled sitemap regex; `ssllabs-scan` Go wrapper (HTTPS covered by stdlib + PageSpeed); vendoring `validator.schema.org` (no API, link out instead); Rich Results Test API (none public — link out + Search Console inspection); `jimsmart/robotstxt` (repo gone from GitHub); `ikeikeikeike/go-sitemap-generator` for generation (last tag `v1.0.2`, 2019-03-26 — generation is own-code or dropped).

## Whole-site Go SEO checkers (reference for the decision table)

Survey 2026-09-19. Ordered by fit for `dev seo`. Names are stable; earlier W-numbers removed (they drifted on insert).
Pure-Go CLI means: one `go install` / `go build`, no DB, no node/react build, no docker-compose to run.

| Tool | Links | Shape | Pure Go CLI? | What it does | Verdict |
|---|---|---|---|---|---|---|
| `muffet` | [repo](https://github.com/raviqqe/muffet) · [docs](https://raviqqe.github.io/muffet/) | Single `main.go` binary, MIT, 2.6k stars / 111 forks / 26 contributors, `v2.11.5`, Go 1.26, pushed 4 days ago, 67 releases, goreleaser | Yes — `go install github.com/raviqqe/muffet/v2@latest`, `muffet <url>`, text/JSON/JUnit, GitHub Action + Docker | Recursive link checker: `a/img/link/script` tags, robots.txt (`temoto/robotstxt`), sitemap-as-root, throttling, retries, cookies, IPv6. Link layer only — no title/meta/H1/JSON-LD/CWV. | Best link layer. Mature, tiny dep tail (`fasthttp`, `temoto/robotstxt`, `x/net`). Shell as binary or borrow fetcher/throttler; pair with an audit core. |
| `scoutly` | [repo](https://github.com/nelsonlaidev/scoutly) · [pkg audit](https://pkg.go.dev/github.com/nelsonlaidev/scoutly/audit) | Go CLI + TUI + importable Go library, MIT, `v0.5.0` (2026-09-05), Go 1.26.6, 7 releases | Yes — `go install ./cmd/scoutly`, `scoutly <url> [--format json]`, plus `audit.Audit(ctx, url, opts, progress)` lib. TUI needs a TTY; non-TTY prints report to stdout. Has `npm/` installer shim but core is pure Go. | Same-origin BFS crawl, broken/redirected links, image checks (`src/srcset/picture/og:image`, status+content-type), titles/meta/H1/alt/thin-content/OG, robots.txt respected (512 KiB cap), sitemap index+urlset+gzip (50 MiB/50k caps), config file + per-rule levels, text/JSON reports | Best audit core. Shell as binary (see decision table — lib import drags the charm TUI tail). JSON-LD depth + CrUX added beside it. |
| `seo-audit` (Erose112) | [repo](https://github.com/Erose112/seo-audit) · [docs](https://github.com/Erose112/seo-audit/tree/main/docs) | Go CLI (Cobra), MIT, pushed 2026-09-16, Go 1.27+, 0 stars | Yes — `go install`, `crawl/compare`, `--output json/text`, `--fail-below`, `--baseline` | 9 checks (title, length, meta, SINGLE_H1, IMAGE_ALT, CANONICAL, VIEWPORT + DUPLICATE_TITLE + BROKEN_LINK site-wide), 0–100 scoring, robots.txt honoring, exit contract 0 pass / 1 quality-gate / 2 crawl-error, offline fixtures | Best CI-gate contract to copy for `dev seo` (baseline, strict rules, stdout-pure JSON — but **not** the exit codes: `cli` owns 2 for usage errors, so `dev seo` is 0/1, see the Gate row). Too narrow alone (v1 explicitly excludes schema.org, JS rendering, sitemap intelligence, redirect chains). |
| `cruxpeek` | [repo](https://github.com/printemps-tokyo/cruxpeek) | Stdlib-only Go CLI, MIT, 2026-07-24, Go 1.23+ | Yes — `go install`, `cruxpeek [--json/--strict/--history] <url>`, key only from `CRUX_API_KEY` | CrUX API field data: p75 LCP/INP/CLS (+FCP/TTFB), official web.dev thresholds, exit 0 pass / 1 poor / 2 key / 3 no-data, offline fixture tests | Best CWV sidecar. Stdlib-only means copy the pattern into `internal/seo` (or shell the binary) instead of pulling `google-api-go-client` for CrUX alone. Keep PSI client (#5) only for lab data. |
| `brokli` | [repo](https://github.com/EndlessTrax/brokli) · [releases](https://github.com/EndlessTrax/brokli/releases) | Go CLI (Cobra), MIT, `v0.2.1`, Go 1.24+, 1 star, author's coverage claim (unverified here) | Yes — `brokli check url|sitemap <url> [-o verbose/github]`, GitHub Action annotations + step outputs | Concurrent link checker (10 workers), color/progress output, sitemap check with `lastmod` display, configurable workers/timeouts/redirects/UA | Good link checker w/ CI annotations. Younger than muffet (4 releases vs 67). Pick one link layer: muffet wins (decision table). |
| `CrawlGrade` | [repo](https://github.com/pan-dolina/CrawlGrade) | Go CLI (Cobra + `x/net` only), Apache-2.0, pushed 2026-09-15, under development, 0 stars | Yes — `cmd/crawlgrade`, no DB, no frontend | Local crawl within safety limits; crawlability, metadata, content, structured data, internal link graph, hreflang/OG/Twitter, passive hygiene; ARCHITECTURE.md + DEVELOPMENT_PLAN.md | Watch. Clean scope match, smallest dep tail (cobra + x/net), but pre-release, single author, no releases. Re-check after milestones land. |
| `gopherseo` | [repo](https://github.com/TarikTz/gopherseo) · [releases](https://github.com/TarikTz/gopherseo/releases) | Go CLI (Colly+Cobra+goquery), MIT, `v0.2.0` (~2026-02), 0 stars | Yes — `gopherseo crawl <url> [--threads/--depth/--exclude]`, goreleaser | Recursive internal crawl, `sitemap.xml` generation, broken-link + canonical reports (missing/multiple/cross-domain/chains), robots via Colly; roadmap: meta/OG, robots analysis, CWV, schema.org, HTML/JSON | Narrow today (sitemap + links + canonical). Roadmap is the rest. Useful Colly wiring reference; not the base. |
| `go-seo-analyser` (mzahan) | [repo](https://github.com/mzahan/go-seo-analyser) | Go CLI, 0 stars, 5 months ago, Go 1.26 | Mostly — flags + `-render-js` via `go-rod/rod` (needs Chrome at runtime, auto-managed) | Per-page HTML (title, meta, H1/levels, canonical, robots/noindex, viewport, `html[lang]`, alt, readability, status) + site-level robots + `llms.txt` probes (AI policy) | Small and readable; `llms.txt` probe is the only new idea vs the rest. Rod/Chrome pulls it out of pure-CLI. Not the base. |
| `deadsniper` | [repo](https://github.com/port19x/deadsniper) | Single-file Go CLI + GitHub Action (`action.yml`), 8 stars, Go 1.22 | Yes — one `deadsniper.go`, action wrapper | Fast dead-link checker, CI-oriented | Minimal link checker. No releases, single file. Only if muffet is rejected. |
| `micelio` | [repo](https://github.com/jlhernando/micelio-crawler) · [releases](https://github.com/jlhernando/micelio-crawler/releases/latest) | Single-binary Go CLI + embedded Svelte dashboard (`go:embed`), MIT, `v1.0.0` (~2026-07), Go 1.26, chromedp for `--js` | No — needs node 20+ to build dashboard, SQLite store, optional Chrome/keys; `package.json`, `dashboard/`, `src/` TS alongside Go | `crawl/list/sitemap/head/robots-test/verify-bot/diff/generate-sitemap/schedule/ui`; full SEO + link intelligence + content + `--psi/--crux` + log analysis + GSC/GA4 | Closest to the whole brief but not pure CLI. Do not vendor; use as binary via releases or borrow taxonomy + flag shapes (`--psi/--crux/--gsc`, `robots-test`, `verify-bot`, `diff`). |
| `seonaut` | [repo](https://github.com/StJudeWasHere/seonaut) · [site](https://seonaut.org) | Go web app + MySQL + Docker, MIT, 787 stars / 133 forks, pushed 2026-05-23 | No — server + DB + compose | Full site scan, severity model (critical/high/low), dashboard, WACZ export | Most mature, wrong shape. Reference taxonomy only. |
| `seo-automation` | [repo](https://github.com/Piyush8296/seo-automation) | Go + React + Docker, 1 star, pushed 2026-04-28, Go 1.22+ | No — node-built React UI, `serve` mode, `vendor/` committed | Claims 175+ checks / 22 categories, A–F score, `audit/serve/diff/check-exit`, daily Action | Best checklist source. Stale 5 months. Borrow checklist, not code. |
| `seo-auditor` (igor-zatochniy) | [repo](https://github.com/igor-zatochniy/seo-auditor) | Go service + PostgreSQL + Docker, MIT, pushed last week | No — DB service | Concurrent pool, RFC 9309 robots, SSRF guard, streaming tokenizer, HTML report | Borrow robots/SSRF/streaming notes only. |
| `Audit-Ant` | [repo](https://github.com/UrviPatil6/Audit-Ant) | Go CLI set + Node/Playwright renderer + Looker Studio, 0 stars | No — `renderer/` node/Playwright, multiple bins | 25+ checks, resumable crawls, headless CWV without key, axe-core, diff, monitors | Broad but unproven. Headless-CWV-without-key idea worth noting for fallback #8. |

Superseded by the decision table above (shell scoutly+muffet, copy cruxpeek pattern + Erose112 contract, JSON-LD + lab PSI beside them; micelio/seo-automation taxonomy as reference; CrawlGrade as watch).

## Recency (2026-09-19)

Latest tag date from the proxy, plus last push on the repo. Verdict is keep / pin-newer / drop.

| # | Module | Links | Latest tag (proxy time) | Last push | Verdict |
|---|---|---|---|---|---|
| 1 | `temoto/robotstxt` | [repo](https://github.com/temoto/robotstxt) · [pkg](https://pkg.go.dev/github.com/temoto/robotstxt) | `v1.1.2` (2021-03-31) | 2026-05-25, not archived, 288 stars | Keep with caution. Tag is old but repo is alive; REP (RFC 9309) is stable so staleness is low-risk. Re-check `Test()` semantics in spike; fallback is own-code on `x/net` if it misbehaves. |
| 2 | `snabb/sitemap` | [repo](https://github.com/snabb/sitemap) · [pkg](https://pkg.go.dev/github.com/snabb/sitemap) | `v1.0.5` (2026-04-18) | 2026-04-18, 48 stars | Keep. Current. Small scope (parse only) is a plus. |
| 3a | `PuerkitoBio/goquery` | [repo](https://github.com/PuerkitoBio/goquery) · [pkg](https://pkg.go.dev/github.com/PuerkitoBio/goquery) | `v1.13.0` (2026-08-27) | 2026-09-14, 15k stars | Keep. Current and heavily used. |
| 3b | `gocolly/colly/v2` | [repo](https://github.com/gocolly/colly) · [pkg](https://pkg.go.dev/github.com/gocolly/colly/v2) | `v2.3.0` (2025-12-04) | 2026-09-16, 25k stars | Keep. Current; note latest GitHub release tag is `v2.2.0` while proxy has `v2.3.0` — pin the proxy version after `go doc` in spike. |
| 4a | `piprate/json-gold` | [repo](https://github.com/piprate/json-gold) · [pkg](https://pkg.go.dev/github.com/piprate/json-gold) | `v0.8.0` (2026-02-23) | 2026-09-12, 314 stars | Keep. Current. |
| 4b | `santhosh-tekuri/jsonschema` | [repo](https://github.com/santhosh-tekuri/jsonschema) · [pkg v6](https://pkg.go.dev/github.com/santhosh-tekuri/jsonschema/v6) | `v6.0.3` (2026-06-28) | 2026-08-06, 1.3k stars | Keep, but pin `v6` not `v5`. Plan previously said `v5` (`v5.3.1`, 2023-07-22) — superseded. |
| 5+6 | `google.golang.org/api` | [repo](https://github.com/googleapis/google-api-go-client) · [pkg pagespeed](https://pkg.go.dev/google.golang.org/api/pagespeedonline/v5) · [pkg searchconsole](https://pkg.go.dev/google.golang.org/api/searchconsole/v1) | `v0.298.0` (2026-09-14) | 2026-09-17, weekly releases | Keep. Current by construction; repin at spike time. |
| 3/7 | `golang.org/x/net` | [repo](https://github.com/golang/net) · [pkg html](https://pkg.go.dev/golang.org/x/net/html) | `v0.59.0` (2026-09-08) | 2026-09-18 | Keep. Current; already implied by Go toolchain. |
| — | `ikeikeikeike/go-sitemap-generator` | [repo](https://github.com/ikeikeikeike/go-sitemap-generator) | `v1.0.2` (2019-03-26) | 2024-07-10 | Drop from generation slot. 7 years since tag; generation (if needed at all) is own-code. Parsing stays with #2. |

## Fit to `dev`

- **New `internal/seo/`** — one thing: run the pinned binaries, merge their JSON, apply the gate. Binaries are found the way every other external tool here is found: a `ScoutlyBin`/`MuffetBin` const, `exec.LookPath`, and an error naming the `mise.toml` line to add (`internal/stage/browser.go:81-83` is the model, `internal/cloudflare/cloudflare.go:25` the other). `internal/seo` brings its own small runner because it must capture stdout; `stage.run()` is unexported and streams everything to stderr by design, since stdout is for data (`internal/stage/stage.go:51-62`). `usage.md` beside the verbs, `go:embed` like `internal/stage/verb.go`.
- **No new `go.mod` entries.** `dev` stays at two direct deps. Every tool is a binary pin in `mise.toml [tools]`; the JSON-LD step is own code. v1 does not import `json-gold`/`jsonschema` — there is no `schema.org` JSON Schema to validate against without hand-writing one, so that is a separate decision rather than a step.
- **New verb in `main.go`**: `dev seo URL [--strict] [--fail-below N] [--baseline PATH]` — the `URL`-first shape `wait` already has (`main.go:55`), so nothing new is asked of `cli`. Offline v1: scoutly + muffet + own-code JSON-LD. Online v2 adds CrUX (cruxpeek's stdlib pattern) + lab PSI, skipped without creds.
- **Exit codes are `cli`'s, not Erose112's.** 0 pass, 1 fail. `cli` maps any returned error to 1 and a `*UsageError` to 2, and a `Verb.Run` returns only an `error` — it cannot pick a code (`cli/command.go:66`, `cli/command.go:141-146`). Whether a run failed the quality gate or failed to crawl at all is said in the message and in the JSON (`"outcome": "gate"|"error"`), never in the exit status. Widening `cli` to carry exit codes would change the public API every repo on the stack builds on; it is not part of this plan.
- **Pipeline placement: after deploy, in its own task.** `validate` is what `deploy` runs *first* (`skill.md:36`), so a check needing a live URL cannot live there — on a first deploy there is nothing to crawl, and after that it would grade the previous deployment. `check` is pre-deploy for the same reason. A consumer repo wires `<cmd>:seo` as `dev seo $(dev url cmd/<name>)` to run after `deploy`, chained the way `wait` is. `check` stays `go test + release --snapshot`.
- **The binaries have to exist in the consumer repo, not just here.** The pins land in *this* repo's `mise.toml`, but the verb ships to every repo that pins `dev` through packslip, where a bare `exec: "scoutly": not found` would be the whole message. So a missing binary fails with the exact pin line to add, and `internal/seo/usage.md` prints that line — the contract node, wrangler and flyctl already keep.
- **Tests stay hermetic.** This tree has no `t.Skip` anywhere, and it replaces external tools through package variables (`internal/fnox` is the pattern the layout table names). `internal/seo` does the same: a replaceable runner variable, golden JSON captured from the Spike as fixtures, no crawler executed by `go test` and none in CI's critical path. A live end-to-end run, if ever wanted, is opt-in and outside `check`. Where a fixture serves HTML to a tool that brings its own client, `httptest.NewTestServer` needs `srv.Start()` (AGENTS.md).
- **Copied code carries its notice.** cruxpeek (MIT) and Erose112/seo-audit (MIT) are the two “copy the pattern” sources. Where what is taken is a shape — the gate contract, the check taxonomy — re-implement from behaviour; where lines are copied, the MIT notice goes in the file that holds them and names the source.
- **Secrets**: keys only via env from `fnox exec --` (`CRUX_API_KEY`, PSI key), never as flags (flags leak via `ps`), never committed. `usage.md` says so.
- **Who owns the live-site check.** This plan owns crawling a deployed URL for Google conformance. `.plans/2026-09-19_0900_stagehand-for-browser-check.md` owns the in-`check` browser probe and what becomes of `tools/browser-check.mjs`. Tool list row #8 mentions that probe as history only: it is dropped here, so nothing in `internal/seo` depends on node or on how that plan lands.
- **Non-goals**: `llms.txt` / AI-visibility checks are not Google Search conformance — parked, and they must not creep into `usage.md`.
- **Rules kept**: every error names its fix; `usage.md` edited in the same change as a flag; `dev skill` regenerates the manuals; `go test` holds them via `cli.CheckSkill`/`cli.CheckUsage`. `internal/seo/usage.md` needs no `mise.toml` edit — the build `sources` glob `internal/*/usage.md` (`mise.toml:50`) already covers it, which is the stale-binary trap AGENTS.md warns about.
- **Deps discipline**: exact pins (`/v2` for muffet, lowercase module paths for gopherseo/brokli if ever used, `./cmd/...` subpaths for crawlgrade/go-seo-analyser), `mise run check` not `build`, one file at a time + `go build ./...`, commit green first.

## Plan

- [x] 0. Spike: all 15 candidates pinned, built, smoke-tested (see Spike). No repo changes.
- [x] 0b. Review against this repo rather than against the tools (see Review 2).
- [x] 1. `internal/seo/`: the runner, the checker registry, golden fixtures, hermetic tests.
- [x] 2. Own-code JSON-LD presence checks in the head validator. Zero new deps.
- [x] 3. The gate: `--fail-on error|warning|info`, `--json`, `--out`, `--record` with drift. Exit 0 pass / 1 fail.
- [x] 4. The `seo` verb, `usage.md`, the pins. `dev skill` regenerates; `go test` green.
- [x] 5. Post-deploy task shape documented in `usage.md`. Never `validate`, never `check`.
- [ ] 6. v2: CrUX + lab PSI behind `fnox` env, skipped without creds.

**Built beyond this plan**, because the work found them:

- [x] **A write side.** `dev seo write DIR` produces sitemap.xml, robots.txt and
  head.html, and `dev seo validate DIR` checks them with no network. Each
  writer declares the check that proves its own output good, so the two cannot
  disagree. All own code: no Go package generates robots.txt, and the packages
  that generate the rest each cost a dependency this tool will not carry.
- [x] **The report belongs to `cli`, not to seo.** `cli.Report`, `Finding`,
  `Step`, severities, `--fail-on`, `--record`/drift, `Parallel` with a
  deterministic merge — so any checking verb on the stack answers the same way.
- [x] **`cli/tool`**: one runner for every external binary on the stack, 21
  call sites converted, with the mise pin quoted when one is missing.
- [x] **Findings name the writer that fixes them.** A checker reports
  `seo.canonical.missing`; the report says `head.html` closes it and prints the
  command. That is the loop the two halves exist to close.
- [x] **kitsune** added as a fourth checker for its stable dotted ids.

## DOD — met, except where marked

**v1, all offline or against a deployed URL:**

- [x] `mise run check` green; no crawler binary executed by `go test`.
- [x] `dev seo check <url>` fails with a named fix on each finding; verified
  live against `gsxhq.github.io` and `ui.gsxhq.dev`.
- [x] `dev seo write` produces artifacts that its own validator passes, and
  `dev seo validate` agrees — proven by breaking each file and watching the
  matching check fail.
- [x] Exit 0 on pass, 1 on fail; `--fail-on` moves the line; `--record` says
  what changed since the last run.
- [x] A missing checker fails with the exact `mise.toml` line to add.
- [x] No secret in arguments, logs or git; `usage.md` true to the flags;
  manuals regenerated by `dev skill`, never hand-edited.

**v2 — step 6, not built:**

- [ ] A slow-LCP finding through the CrUX path, skipping cleanly without creds.

## Open questions — answered 2026-09-19; three corrected after Review 2

1. ~~`dev seo URL` vs `dev check DIR`?~~ `dev seo URL`, the `wait` shape. **Corrected:** wired as a post-deploy `<cmd>:seo` task — *not* `validate`, which runs before `deploy` (`skill.md:36`), and not `check`.
2. ~~Offline-only v1, or online in the same cut?~~ Offline-only v1 (scoutly + muffet + own-code JSON-LD); online (CrUX + PSI) is v2, behind `fnox`.
3. ~~Keep the `node lighthouse` fallback?~~ Dropped. Go + hosted APIs only; no `npx`, no node path.
4. ~~Copy Erose112's 0/1/2 exit contract?~~ **Corrected: no.** Exit 0/1 only. `cli` owns 2 for usage errors and gives a verb no way to choose a code (`cli/command.go:66`, `141-146`); gate-vs-error goes in the JSON.
5. ~~Import `json-gold` + `jsonschema/v6` for JSON-LD?~~ **Corrected: not in v1.** Own code, zero deps. Real schema.org validation needs a schema nobody publishes, so it is a later decision with its own evidence.

## Spike (2026-09-19, `go 1.27.1` via `mise which go`)

Scratch modules outside the repo (`/tmp/seospike`, `/tmp/seospike2`, `/tmp/seospike3` — deleted after). Outside the repo there is no mise Go shim (same trap as the stagehand plan step 0); used the repo-pinned binary by absolute path. Overlaps kept deliberately: no one tool covers robots + sitemap + on-page + JSON-LD + CWV + gate, so each layer gets its own check below.

### Single-purpose libs — all resolve, build, APIs confirmed

Scratch `seospike`: `go get` all six at exact pins, `go build` clean, `go doc` entry points read. Full-module tail `go list -m all`: 44 modules.

| # | Pin | License | API confirmed | Note |
|---|---|---|---|---|
| 1 | `temoto/robotstxt v1.1.2` | MIT | `FromString/FromBytes/FromResponse/FromStatusAndBytes`, `(r *RobotsData).TestAgent(path, agent)`, `FindGroup`, `Sitemaps []string` | Entry point is `TestAgent`, not `Test`. `FromStatusAndBytes` handles the `4xx`=allow / `5xx`=fail-closed rule in own code. |
| 2 | `snabb/sitemap v1.0.5` | MIT-style (Janne Snabb) | `New()`, `ReadFrom(r)`, `WriteTo(w)`, `Add(u)`, `NewSitemapIndex()` | Generate + read in one lib. No `Parse` symbol — read path is `ReadFrom`. Index support via `SitemapIndex`. |
| 3 | `PuerkitoBio/goquery v1.13.0` + `x/net v0.58.0` (pulled; `v0.59.0` latest) | BSD-style (Martin Angers) | `Selection.Find(selector)` et al | As expected. `x/net` resolved `v0.58.0` under these pins; repin `v0.59.0` at implementation. |
| 3b | `gocolly/colly/v2 v2.3.0` | Apache-2.0 | `NewCollector(opts...)`, `OnHTML/OnResponse/OnResponseHeaders`, `Limit/Limits`, `SetRequestTimeout`, `Visit` | Full crawl engine confirmed. Overlaps scoutly/muffet crawlers — one-crawl-owner decision still open (review finding 1). |
| 4a | `piprate/json-gold v0.8.0` | Apache-2.0 | `ld` subpackage (`Expand`/`Flatten`/`Compact` live there, not root) | Needs `go get github.com/piprate/json-gold/ld@v0.8.0` for the `ld` sum entries (`cayleygraph/quad`, `pquerna/cachecontrol`). Root `go get` alone leaves `go list ./...` short. |
| 4b | `santhosh-tekuri/jsonschema/v6 v6.0.3` | Apache-2.0 | `NewCompiler()`, `AddResource/Compile/MustCompile`, `AssertFormat/AssertContent` | `v6` path confirmed; `v5` superseded as planned. |

### Whole-site Go CLIs — all build under 1.27.1, all smoke-tested

| Tool | Pin resolved | License | Build | `--help` | Live on `example.com` |
|---|---|---|---|---|---|
| `muffet` | `github.com/raviqqe/muffet/v2 v2.11.5` (`/v2` path — pin with it) | MIT | OK (9.5 MB) | OK (`--follow-robots-txt`, `--max-connections`, exclude/include, status-code ranges) | OK: `failed to fetch robots.txt: 404`, exit 0. Correct 4xx handling visible. |
| `scoutly` | `v0.5.0` | MIT | OK (17.8 MB; needed `go get ./cmd/scoutly@v0.5.0` for kong/toml/rate sum entries) | n/a (ran directly) | OK: `--format json --max-pages 5` returned full report — 1 page, `missing-meta-description` error + `redirect` info, title/H1/OG shapes as documented. `audit.Audit(ctx, target, opts, progress)` signature confirmed via `go doc`. |
| `seo-audit` (Erose112) | `v0.0.0-20260916165401-2b1e6fc29596` (no tag — pseudo-version) | MIT (repo file) | OK (13.6 MB) | OK (`crawl/compare`) | OK: `crawl --url --max-pages 5` scored 85/100, named title-length + missing-meta + missing-canonical. Gate contract as documented. |
| `cruxpeek` | `v0.0.0-20260724002006-fa6ae369c9eb` (no tag — pseudo-version) | MIT | OK (9.5 MB, stdlib-only confirmed — no extra `go get` needed) | OK (`--origin/--device/--metrics/--history/--json/--strict`) | Not run (needs `CRUX_API_KEY`; key-via-env-only confirmed from help + README). |
| `brokli` | `github.com/endlesstrax/brokli v0.2.1` (lowercase path — `EndlessTrax` 404s on `go get`; case trap) | MIT | OK (9.8 MB) | OK (`check url/sitemap`, `-o verbose/github`) | Not run live (help + coverage claim verified by reading, not executed). |
| `CrawlGrade` | `v0.0.0-20260915105515-584274e48e81` (no tag) | Apache-2.0 | OK via `./cmd/crawlgrade` subpath | OK (safety-limits copy confirmed) | Not run live (pre-release watch). |
| `gopherseo` | `github.com/tariktz/gopherseo v0.2.0` (lowercase path — same case trap as brokli) | MIT | OK (20.5 MB, largest — Colly tail) | OK (crawl/sitemap/canonical copy confirmed) | Not run live. |
| `go-seo-analyser` | `v0.0.0-20260405205110-95615138beca` (no tag) | ? (no LICENSE file found via API file list) | OK via `./cmd/seo-analyser` after `go get ./internal/render@…` for `go-rod/rod` sum entries | OK (`-crawl/-json/-respect-robots/-render-js` flags confirmed) | Not run live (Rod pulls Chrome at runtime — confirms non-pure-CLI verdict). |
| `deadsniper` | `v0.0.0-20240916053727-7cb4bc3e003b` (no tag, oldest pin here — 2024-09) | ? (single file + action.yml, license file named `license` lowercase — content not read) | OK (8.7 MB, smallest) | OK (`<link to sitemap.xml>`, single purpose confirmed) | Not run live. |

Module-path case traps (found by doing, not reading): `gopherseo` and `brokli` declare lowercase module paths (`github.com/tariktz/gopherseo`, `github.com/endlesstrax/brokli`) while the GitHub owners are mixed-case (`TarikTz`, `EndlessTrax`). `go get` with owner case fails; pin the lowercase path. `muffet` needs the `/v2` suffix. `crawlgrade`/`go-seo-analyser` build from `./cmd/...` subpaths, not repo roots.

Dep tails (`go list -m all`): libs scratch 44 modules; CLI scratch 2 (muffet+scoutly+cruxpeek+cobra) 69 modules; CLI scratch 3 (six smaller CLIs) 57 modules. Scoutly's charm/TUI tail is inside the 69 — hence the decision table shells scoutly as a binary instead of importing the `audit` lib (review finding 2 resolved).

Binaries kept at `/tmp/seospike2/bin`, `/tmp/seospike3/bin` for re-test; scratch `go.mod`s deleted, nothing added to this repo.

## Review 1 (2026-09-19) — the tools; all 10 resolved in the decision table

Read end to end against `AGENTS.md`, `go.mod` (2 direct deps), and the stagehand plan's lessons. Each finding kept for the record with its resolution.

1. ~~Two crawl stacks, undecided.~~ Resolved: scoutly owns crawl + on-page; muffet kept as the thorough link layer (overlap deliberate — summary vs thorough); colly/snabb/direct-robotstxt dropped. Decision table records it.
2. ~~Scoutly TUI tail rides on import.~~ Resolved: shell, don't import. Measured 69-module scratch tail with charm inside; `dev` keeps 2 deps, binaries pinned in `mise.toml [tools]`.
3. ~~Go version skew unexamined.~~ Resolved: all 9 CLIs + 6 libs built under `go 1.27.1` in Spike. `/v2` (muffet), lowercase paths (gopherseo/brokli), `./cmd/...` subpaths (crawlgrade/go-seo-analyser) recorded.
4. ~~Tables disagree on what wins.~~ Resolved: decision table added at top; lib/recency/survey tables marked reference.
5. ~~W-numbering drifted.~~ Resolved: numbers stripped everywhere; names only.
6. ~~Muffet dep licenses unchecked.~~ Resolved: fasthttp MIT, go-flags BSD-style, aurora public domain, gopher-parse-sitemap MIT, ratelimit MIT, rootcerts BSD-2 — in decision table.
7. ~~No secret story for key flags.~~ Resolved: env-only via `fnox exec --` (`CRUX_API_KEY` pattern), never flags; stated in Fit and in step 6.
8. ~~No `seo` vs `check`/`validate` placement.~~ Resolved wrongly, and **corrected by Review 2, finding 1**: this said `seo` goes in `validate` “post-deploy”, but `validate` is what `deploy` runs *first* (`skill.md:36`). It is a post-deploy `<cmd>:seo` task of its own; never `validate`, never `check`.
9. ~~Stale claims.~~ Resolved: brokli coverage reworded to "help + coverage claim verified by reading, not executed"; "most-used" softened to observed ubiquity; pins are spike-dated with repin-at-implementation notes.
10. ~~`llms.txt`/AI-visibility creep.~~ Resolved: parked as explicit non-goal in Fit.

Original findings (pre-fix wording):

1. **The plan recommends two crawl stacks and does not say so.** Tool list #3 says crawl with `colly/v2`; revised recommendation says base on `scoutly` `audit` lib (which crawls itself, on `x/net` + `temoto/robotstxt`) plus `muffet` (which also crawls, on `fasthttp` + `temoto/robotstxt`). Three crawlers, two HTTP stacks (`net/http` vs `fasthttp`), none reconciled. Spike step 0 must decide one crawl owner: `scoutly audit` lib owns crawl + on-page (it already caps robots 512 KiB, sitemaps 50 MiB/50k), `muffet` is a second opinion binary or dropped, `colly`/`snabb` fall out if scoutly covers them. Record the dep tail of the chosen one here — this repo has 2 direct deps today and the stagehand plan already warns a 21-module tail is the largest cost after money.
2. **`scoutly` TUI deps ride along even when imported as a lib.** Its `go.mod` pulls `bubbletea/v2`, `huh/v2`, `lipgloss/v2`, `kong`, `go-toml`, `yaml` — all compile into `dev` if `audit` is imported, whether the TUI runs or not. That breaks the bounded-tool rule more than the stagehand SDK did. Spike must `go list -m all` on a scratch import and write the count here; if the tail is heavy, shell `scoutly`/`muffet`/`cruxpeek` as pinned binaries via `stage.run()` (the established pattern) instead of importing.
3. **Go version skew is unexamined.** This module is `go 1.27.1`. Candidates pin `go 1.23` (cruxpeek), `1.24` (brokli), `1.25` (muffet `v2` module line says 1.26.0), `1.26.x` (scoutly, gopherseo, CrawlGrade), `1.27` (Erose112). A `go 1.23`-era module usually builds fine under 1.27, but `muffet/v2`'s `fasthttp v1.74.0` + `x/net v0.59.0` and scoutly's charm stack need a real `go build` in spike, not proxy metadata. Also `muffet` import path is `/v2` — pin `github.com/raviqqe/muffet/v2`, not `muffet`.
4. **Recency table and whole-site table disagree on ordering.** Recency still lists single-purpose libs #1–#4 as keep, while the revised recommendation demotes most of them (colly/snabb fall out if scoutly covers them; `google-api-go-client` only for lab PSI). Either promote the W-table to the decision table and mark #1–#4 as fallback-only, or the next reader will pin both stacks. Suggested: keep one decision table (W1–W4 + JSON-LD #4 + lab PSI #5), move the rest to references.
5. **W-numbering drifted.** W5 `brokli` was inserted between W4 and old-W5, so prose written earlier ("W5/W9 shapes", "W7/W10 are watches", "W1/W6 taxonomy") now points at the wrong rows. Renumber or switch prose to names. Same class of bug as the verb-table drift `cli.CheckSkill` exists to catch.
6. **Missing: license check on `muffet` deps.** `fasthttp`, `jessevdk/go-flags`, `aurora`, `cupaloy` need license eyeball in spike (stagehand plan records pin + license + tail per candidate; do the same here). `CrawlGrade` is Apache-2.0 while the rest is MIT — fine to shell, matters if vendored.
7. **Missing: secret story for `--psi/--crux/--gsc` flags.** Micelio-style flags take keys as args; repo rule is keys via `fnox exec --`, never args, never committed. If `dev seo` shells any binary with a key flag, the key must arrive via env (`CRUX_API_KEY` pattern from cruxpeek is the model) and `usage.md` must say so. Step 3 should state this explicitly.
8. **Missing: `seo` vs `check`/`validate` interaction.** `check DIR` is what CI runs; `validate` is what `deploy` runs first. If `seo URL` needs a deployed URL it cannot run in `check` pre-deploy; if it runs post-deploy it belongs in `validate` or a new `<cmd>:seo` task. Open question 1 asks verb shape but not pipeline placement — answer both together.
9. **Minor: stale claims.** W2 says "7 releases" and `v0.5.0` (2026-09-05) — re-verify at spike time, weekly-release projects drift. W5 brokli "94% coverage" is the author's claim, untested here. `muffet` "67 releases" vs releases page "67" — consistent, keep. `temoto/robotstxt` "most-used" is asserted without numbers — either cite proxy download counts or soften to "candidate".
10. **Minor: `llms.txt`/`AI visibility` scope creep.** W8's `llms.txt` probe and W10's AI-visibility tabs are not Google Search conformance. Fine as noted, but step 1's fixture list (blocked robots, bad sitemap, missing canonical, bad JSON-LD, slow LCP) should stay Google-only; park AI-bot checks as an explicit non-goal or they will creep into `usage.md`.

Highest value before spike was: fix (1) one-crawl-owner, (2) lib-vs-shell with dep count, (4) single decision table. All three done — see decision table.

## Review 2 (2026-09-19) — the fit to this repo; all 10 folded into the decisions

Review 1 read the plan against the candidate tools. This one read it against
`main.go`, `cli/`, `internal/stage/`, `mise.toml` and `skill.md`. Ten findings;
the first four were blocking, because in each the plan stated something the
code contradicts.

**Blocking:**

1. **`validate` is pre-deploy.** The plan put `seo` in `validate` and called that post-deploy. `skill.md:36` is the stack's own contract: "A `validate` task, what `deploy` runs first." A live-URL check cannot live there — a first deploy has nothing to crawl, and later runs would grade the previous deployment. Resolved: a post-deploy `<cmd>:seo` task of its own.
2. **Exit 0/1/2 collides with `cli`.** `cli/command.go:66` documents 2 as a usage error, and `cli/command.go:141-146` is the whole mapping: error → 1, `*UsageError` → 2. `Verb.Run` returns an `error`, so a verb cannot choose a code. Copying Erose112's contract meant changing `cli/` — the public API every repo on the stack builds on — which the Affects line did not mention. Resolved: 0/1, with gate-vs-error in the JSON.
3. **`stage.run()` can do neither job.** It is unexported, and it sets `cmd.Stdout = os.Stderr` on purpose, because stdout is for data (`internal/stage/stage.go:51-62`). So it can neither be called from `internal/seo` nor hand back JSON to parse. Resolved: `internal/seo`'s own runner, using the `<Tool>Bin` + `exec.LookPath` + named-error pattern from `internal/stage/browser.go:81-83` and `internal/cloudflare/cloudflare.go:25`.
4. **Consumer repos would have had no binaries.** The pins were only in this repo's `mise.toml`, but the verb ships to every repo pinning `dev`; there it would fail with a bare `exec: "scoutly": not found`. Resolved: a named error carrying the pin line, and `usage.md` printing it.

**Should-fix:**

5. **The JSON-LD step contradicted the plan's own reasoning, and half of it did not exist.** The plan refused to import scoutly over its dep tail, then imported `json-gold` + `jsonschema/v6` — 2 → 4 direct deps, plus the `ld` tail the Spike itself measured (`cayleygraph/quad`, `pquerna/cachecontrol`) — to validate against "vendored `schema.org` shapes". schema.org publishes no JSON Schema, so those shapes would be hand-written and hand-maintained here. Resolved: v1 is own code with zero deps; validation is a later decision.
6. **The fixtures broke this repo's test habit.** There is no `t.Skip` in this tree; external tools are replaced through package variables. Step 1 as written would have run two network-capable crawlers inside `go test`, and in CI. Resolved: a replaceable runner variable plus golden JSON from the Spike.
7. **The DOD described a version the plan did not build.** "slow LCP via API" is v2 but sat in a v1 DOD, and `--baseline` was in both the verb signature and the DOD with no step implementing it. Resolved: DOD split v1/v2; baseline is step 3.
8. **The header contradicted itself.** Two **Affects** lines disagreed on whether `go.mod` changes, and a stale **Decisions needed before starting** line re-opened the questions the line above it had just answered. Resolved: one header, stale lines deleted.
9. **Copied code had no attribution story.** "Copy the cruxpeek pattern" and "copy the Erose112 contract" are both MIT. Resolved: named in Fit — shapes re-implemented, copied lines carry the notice.
10. **Cross-plan overlap was unnamed.** Tool list row #8 still pointed at `tools/browser-check.mjs`, which the stagehand plan proposes to delete, and both plans add a "check the live app" layer. Resolved: an ownership line in Fit — this plan owns the deployed-URL crawl, that one owns the in-`check` browser probe.

**What held up:** the decision table and its evidence; the Spike's traps (`/v2`, lowercase module paths, `./cmd/...` subpaths, the measured dep tails); the `URL`-first verb shape, which matches `wait` (`main.go:55`); the `[tools]` pin syntax (`mise.toml:12`); and parking `llms.txt` as an explicit non-goal.
