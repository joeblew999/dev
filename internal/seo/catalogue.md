# The SEO catalogue

The inventory behind `dev seo`: what it writes, what it validates, every
checker it runs, how each is invoked, what it returns, what is wrong with it,
and what was rejected and why.

Read with:

- `usage.md` beside this file — what the verb does, and how a repo wires it.
  That one is compiled into the binary and rendered into the dev skill; this
  one is reference and is not.
- `.plans/2026-09-19_1200_google-search-conformance-tools.md` — the survey and
  the decision that produced this.

This document exists because the hard part was never the code. It was finding
out which of these repos work, which lie about their install path, and which
quietly return the wrong answer.

**Provenance is marked throughout.** *Verified here* means it was installed and
run against a real site from this repo. *Reported* means it comes from the
`seocheck` catalogue this document was adapted from and has **not** been
confirmed here — treat it as a lead, not a fact.

---

## Part 0 — The three halves

`dev seo write DIR` produces the files a site needs, `dev seo validate DIR`
checks them with no network at all, and `dev seo check URL` runs the external
checkers against a deployed page. Writing and validating share one
declaration — each writer names the check that proves its own output good, and
`validate` runs that same check — so an artifact cannot pass when it is
written and fail when it is checked.

Nothing is written by a library, and that is a decision rather than an
omission. No Go package generates robots.txt: every one of them is a parser,
which Part 2 records. The packages that generate the rest each cost a
dependency — `teseo` drags `a-h/templ` in to render six meta tags — and this
tool has two dependencies. A sitemap is XML and robots.txt is four lines.

| Written | Checked by |
|---|---|
| `sitemap.xml` | absolute `<loc>` on the site's own host, priority in range, the 50,000-URL and 50MB caps Google enforces |
| `robots.txt` | not blocking Googlebot, an absolute `Sitemap:` directive, the 500 KiB Google reads to |
| `head.html` | one `<title>` under 60 characters, a meta description, an **absolute** canonical, the four Open Graph tags, a JSON-LD block |

Each finding names its fix and the Google page that explains why — the same
rule the live checkers follow, because a code and a severity say what is wrong
and not what to do about it.

## Part 1 — What `dev seo check` runs today

### `kitsune` — the precise ids

```
go install github.com/berkaycubuk/kitsune/cmd/kitsune@latest
kitsune --json <url>
```

*Verified here.* Everything below was run against a live site from this repo.

- **The install path is `/cmd/kitsune`, not the module root.** The root has no
  main package.
- **Stable dotted ids** — `seo.title.short`, `seo.canonical.missing`,
  `geo.jsonld.absent`, `security.csp.missing`. No other checker here has ids
  this precise, and they are what a CI rule should match on.
- 55 findings on a small site, across five categories: `seo` 24, `geo` 11,
  `perf` 11, `security` 7, `a11y` 2. The **geo** category is unusual: it checks
  `/llms.txt` and whether robots.txt names GPTBot, ClaudeBot, PerplexityBot,
  Google-Extended.
- Result fields are `id, category, severity, title, detail, recommendation,
  guideline_url`. **The source catalogue says `docs_url`; it is
  `guideline_url`.** That is the kind of thing that only shows up by running it.
- 39 of 55 findings were `info` — passing checks. Filter them or the report
  floods.
- It fetches `/sitemap.xml` and `/robots.txt` itself, so it catches a missing
  sitemap that a page-only checker cannot see.

Its `seo`, `perf` and `security` categories are taken; `geo` (llms.txt and the
AI crawlers) and `a11y` are not Google Search conformance and are left out.
Nothing is lost by that: kitsune's own report is written beside the merged one
in full, so anyone who wants those has them. That is what keeping each
checker's output is for.

It is the only checker that carries both a recommendation and Google's own
guideline link, so its words are used rather than this package's table.

Seven checkers, pinned in `mise.toml [tools]`, each declared once in the
`checkers` registry in `seo.go`. Every one keeps its own output shape: the
merged report carries only what they all have — an issue, where it is, how to
fix it — and each checker's full output is written beside the report and linked
from it. Nothing is lost to the merge.

### `scoutly` v0.5.0 — crawl and on-page

```
scoutly <url> --format json --max-pages N
```

*Verified here.* Pure JSON on stdout, progress on stderr. Shape:
`{url, audited_at, summary:{pages,links:{total,checked,broken,redirected},
issues:{total,error,warning,info}}, issues:[{code,severity,message,target:{type,url}}],
pages, links, images}`.

- Codes are stable and lowercase-hyphenated (`missing-meta-description`,
  `missing-h1`), which is what `scoutlyFixes` keys on.
- Reports `info` findings (redirects) alongside real ones. They are kept —
  they do not fail the gate, and a redirect is worth seeing.
- The `audit` library is importable, but it drags the charm TUI stack
  (bubbletea, huh, lipgloss, kong) into whatever imports it. Shelled as a
  binary for that reason, measured at a 69-module tail.

### `muffet` v2.11.5 — the link layer

```
muffet <url> --format json --buffer-size 16384 --max-connections 16 --ignore-fragments
```

*Verified here, and three of its four flags are there because of a failure.*

- **`--follow-robots-txt` is fatal on a 404.** muffet stops; Google treats a
  missing robots.txt as allow-all. The flag is deliberately **not** passed.
- **Fragments inflate everything.** Without `--ignore-fragments`, one
  playground link with sixty saved states reported as sixty broken links.
- **The default buffer is smaller than some hosts' response headers.** GitHub's
  headers overflow 4096 and the failure reads as a broken link on a fine page.
- **It lists only pages that have something wrong.** Counting them as "pages
  crawled" reports a clean site as an empty one, so its crawl counts are
  ignored and only its broken links are taken.
- Its errors are sentences, not codes, so `muffetCode` maps them onto names
  (`broken-link`, `slow-link`, `insecure-link`, `unreachable-link`) — one
  failure gets one name however it was worded.

### `seo-audit` (Erose112) — the scored checks

```
seo-audit crawl --url <url> --output json --max-pages N
```

*Verified here.* Shape: `{schema_version, url, score, pages_crawled,
checks:[{check_id,name,severity,pages_failed,pages_total}], pages, site_issues,
summary}`.

- A check carries **how many pages failed it**, rather than a page carrying
  what it failed — a third shape again, and the reason `Found` exists.
- Its ids SHOUT (`TITLE_LENGTH`, `META_DESCRIPTION`); `seoAuditCodes` maps them
  onto the same names scoutly uses, so one failure is one name however many
  checkers found it, and the advice written once is reached from all of them.
- Only checks with `pages_failed > 0` become issues. A check every page passed
  is not news.
- Its **site-wide score and `site_issues`** (duplicate titles across pages) are
  exactly what the merged report cannot model — read them in the sub-report.

### `scry` — the broad one

```
scry check <url> -o json
```

*Verified here, and it corrects the source catalogue twice.*

- The catalogue says it emits no rule ids, every issue carrying
  `rule_id: null`, and that its findings collapse to `scry.unnamed`. **There is
  no `rule_id` field at all.** The id is `check_name`, populated on every one
  of 23 issues, and it reads `category/name` —
  `seo/missing-meta-description`, `security/missing-x-content-type-options`.
  That is a perfectly good id for a CI rule.
- The catalogue says JSON needs `--output-file`. **`-o json` writes to
  stdout**, which is what this uses; its logs go to stderr where they belong.
- Its category is the first half of the check name. `accessibility` is dropped
  for the same reason kitsune's is: a different subject with its own tools.
- It is the only checker here that sees TLS expiry and the security headers.

### `ldlint` — the schema.org vocabulary

```
ldlint <url>
```

*Verified here.* Text output: a count line, an `ERROR` line per problem, exit
1 when it found any.

It validates JSON-LD against the **vocabulary**, not merely its presence — a
property that is not valid on the type it is attached to. That is the one
check this package could not write: schema.org publishes no JSON Schema, so
validating it means carrying the vocabulary, which is exactly why importing a
validator was not worth it and running one is.

### `icanhasrobot` — Google's own matcher

```
icanhasrobot <robots.txt-file> Googlebot <url>
```

*Verified here.* It ships in `jimsmart/grobotstxt`, a native Go port of
Google's C++ robots.txt parser that preserves its behaviour and its test
suite. Shelled rather than imported, so the authority costs no dependency.

- **Authority, not adoption.** `temoto/robotstxt` has 314 importers against
  grobotstxt's 3 — colly, muffet and GoToSocial all use it — but it is an
  independent reading of REP, and implementations differ at exactly the edges
  that matter. Verified here: with `Disallow: /private/` and
  `Allow: /private/public.html`, Google's matcher allows the second, because
  longest-match wins. And it correctly reports that Google's own robots.txt
  blocks `/search`.
- **It takes a file, not a URL**, so the rules are fetched first — as
  Googlebot, because what matters is what Google is served. That is what
  `Checker.Fetch` is for.
- **Match by pattern, not position.** With no rules it prints `ALLOWED` and
  then a notice explaining why; reading the last line called that no verdict.
- A site with no robots.txt is allow-all, which is fine — but it names no
  `Sitemap:` either, and that is reported, because it is how Google finds the
  list.

---

## Part 2 — Candidates, not yet run

### Reported but unverified here

Each of these comes from the `seocheck` catalogue. None has been installed from
this repo; the gotchas are recorded so nobody re-discovers them the hard way.

| Tool | What it adds | Reported gotcha |
|---|---|---|
| `codebygk/crawlens` | site-wide duplicate titles, orphan pages, redirect chains | Cannot be `go install`ed — `go.mod` declares a bare `module crawlens`. **~100s on a 120-page site.** JSON key is `average_seo_score`, not `average_score`. |
| `tc4dy/HuntCat` | crawl with 4D scoring; reached 120 pages where gopherseo found 64 | **No `go.mod` at all**, and **no JSON mode** — numbers are scraped from an ANSI terminal table. Match the field ending in `%`, not a fixed position. |
| `tariktz/gopherseo` | crawl plus sitemap generation | Module path is lowercase while the GitHub org is `TarikTz`. **`--depth 0` means unlimited**; at `--depth 2` it silently wrote a 4-URL sitemap while reporting 64 found. Its canonical report contradicts itself. |
| `temoto/robotstxt` | a second, independent robots opinion — valuable only to *disagree* with grobotstxt, which means the file is ambiguous | Tag is from 2021, repo alive. |
| `aafeher/go-sitemap-parser` | strict sitemap conformance: relative paths, cross-host URLs, out-of-range priority | `SetStrict` takes a bool. With no URL given, infer the host from the first `<loc>` or every URL fails a spurious host check. |
| `aafeher/go-microdata-extract` | reads seven metadata syntaxes back out of HTML | **`og:image` comes back as a nested array of objects**, not a string. Typing the map as `map[string]string` silently drops it and reports a correctly-tagged page as missing. |
| `pagespeedonline/v5` | Google's own Lighthouse SEO score | Free API key, no OAuth. **Filter audits by `ScoreDisplayMode`** — `notApplicable`, `informative` and `manual` score 0 or null with nothing wrong. |
| `searchconsole/v1` | whether Google **actually indexed** it, and which canonical Google chose | **Use `Verdict`, not `CoverageState`.** Google's string for a page it refused is "Crawled - currently not indexed", which *contains* "indexed" — a substring match reports success for a refusal. |

---

## Part 3 — Rejected

Do not re-litigate these. Each was installed or inspected, here or by the
source catalogue.

| Repo | Why not |
|---|---|
| `gocolly/colly/v2` | A crawl engine we would own and maintain, redundant with scoutly's. One crawl owner. |
| `snabb/sitemap` | scoutly already covers sitemap index, gzip and the 50k cap for checking. Worth keeping in mind if `dev` ever *writes* a sitemap. |
| `temoto/robotstxt` direct | Both binaries bundle it; own code only needs the status-code rules. |
| `EndlessTrax/brokli` | Pick one link layer. muffet wins on 67 releases against 4. Module path is lowercase while the org is mixed-case. |
| `go-seo-analyser` | No LICENSE file found; Rod pulls Chrome at runtime, which breaks a pure-CLI check. |
| `deadsniper` | Single file, no releases. Only if muffet were rejected. |
| `seonaut`, `seo-auditor`, `Audit-Ant`, `micelio`, `seo-automation` | Server, database, node or Playwright shapes. Borrow their check taxonomy; never depend on them. |
| `CrawlGrade` | Cleanest scope match and the smallest dependency tail, but pre-release with no tags. Watch. |
| `qor/seo` | *Reported:* 44 importers and the most-used Go SEO package — and unusable. Pulls gorm v1, qor/admin, MySQL **and** Postgres drivers. |
| `golazy.dev/lazyseo` | *Reported:* the best API of any metadata library. Adding it to an empty module pulled **93 modules** — gRPC, protobuf, OpenTelemetry, Prometheus — to render meta tags. |
| `piprate/json-gold` + `jsonschema/v6` | Considered for JSON-LD validation and **deferred**: schema.org publishes no JSON Schema, so the shapes would be hand-written and hand-maintained here. It would also take `dev` from two direct dependencies to four. |

---

## Part 4 — Adding a writer

One entry in `writers`, naming the file it produces and the check that proves
it good. `write` walks that list and so does `validate`, so the new file is
validated from the moment it exists — there is no second place to remember.

```go
{
	Name:     "manifest",
	Provides: "site.webmanifest — what an installed copy is called",
	Produces: Artifact{Name: "site.webmanifest", Validate: validateManifest},
	Write:    writeManifest,
}
```

## Part 5 — Adding a checker

The registry is the source of truth. One entry, one file, nothing else changes:
the report, the timing, the sub-report, the counts by tool and the failure path
all pick it up.

```go
var mytool = Checker{
	Name: "mytool",
	Pin:  `"go:github.com/me/mytool" = "v1.2.3"`,
	Args: func(url string, pages int) []string {
		return []string{"scan", url, "--format", "json"}
	},
	Read: func(res tool.Result) (Found, error) {
		out, err := res.JSON[myShape]("mytool's report")
		if err != nil {
			return Found{}, err
		}
		return Found{Pages: out.Pages, Issues: ...}, nil
	},
}
```

Then add it to `checkers` and its pin to `mise.toml [tools]`. That is the whole
change.

Rules that hold for every checker:

- **Never shell out yourself.** Everything goes through `cli/tool`, which owns
  binary lookup, the pin quoted in the error when it is missing, stdout capture,
  timing and process control. There is one `exec.CommandContext` on this stack
  and it belongs there.
- **Declare only the fields you read.** `encoding/json` ignores the rest, and a
  field nobody uses is a build break when the tool renames it.
- **Map the tool's vocabulary onto this package's.** One failure gets one name
  however many checkers found it, so the advice written once is reached from
  all of them.
- **Every issue carries its fix**, and a code with no entry still reports, with
  the tool's own message and the documentation index. A checker that hides what
  it cannot explain is worse than one that admits the gap.

---

## Part 6 — The route out

A report that only lists faults leaves the reader to work out what to do. Each
writer declares the finding ids its file resolves — matched by prefix, so
kitsune's `seo.canonical.*` and scoutly's `missing-canonical` both land on the
same writer — and every finding carries `fixedBy` naming it. The report then
ends with the count and the command:

```
13 of 28 findings are files this can write:
  head       head.html        fixes 11
  sitemap    sitemap.xml      fixes 2

  dev seo write <dir> --url https://ui.gsxhq.dev --title "..." --desc "..."
```

A finding with no `fixedBy` is content, not configuration: a broken link is
something to fix in the page or upstream, and saying that a file will not fix
it is worth as much as saying which one will.

`TestWritersOnlyClaimWhatTheyCanFix` holds the claims to the routing, so a
writer cannot promise a fix the report will not send to it.

## Part 7 — Invariants

- **stdout carries the report** and nothing else. Progress, per-tool timing and
  warnings go to stderr, so `dev seo <url> --json | jq` works.
- **The file and stdout are the same bytes.** `--out` adds a destination; it
  never changes what stdout receives.
- **Every checker appears in the report**, including one that did not run, with
  the reason. An absence is never silent, and one unpinned tool is not a reason
  to learn nothing about the page.
- **Nothing a checker said is rewritten.** A finding carries the id the
  checker emitted — scoutly's `missing-canonical`, kitsune's
  `seo.canonical.missing`, scry's `seo/missing-canonical`, seo-audit's
  `CANONICAL` — because a report that translates what a tool said is a report
  you cannot check against the tool, and a CI rule matches on what the tool
  emits. The advice written once is *found* through a mapping; the id is not
  changed by it. Several checkers reporting one fault four ways is fine and
  worth seeing: where they agree, the problem is not in doubt.
- **Every byte a checker produced is kept** in `.reports/<name>/<tool>.json`,
  written *before* it is parsed — so a checker this package cannot read still
  leaves everything it said where a person can look. JSON is re-indented and
  nothing else is touched: one tool pretty-prints and the next emits a single
  thousand-character line, and a file nobody can read is a file nobody reads.
  An empty one is an answer too — muffet prints only the pages with something
  wrong, so `[]` means no broken links, and the report says so in words.
- **Exit 0 pass, 1 fail, and `--fail-on` decides which.** The default is that
  errors fail and warnings do not, so the same command is a report on a laptop
  and a gate in CI. 2 belongs to `cli` for usage errors, and a verb cannot
  choose a code; whether a run failed the gate or failed to crawl is in the
  report's `outcome`, not the exit status.
- **A bad flag fails before the work starts.** A crawl takes seconds to
  minutes, and failing afterwards on a typo in `--fail-on` wastes every one of
  them. A typo is answered with what was meant.
- **What is written validates.** `write` runs the same checks `validate` does,
  on what it just wrote, so the two can never disagree.

---

## Part 8 — The toolchain, as this repo runs it

`mise run check` (tests plus a signed snapshot release), `mise run lint` (the
hk list: gofmt, go vet, gomod tidy, staticcheck, trailing whitespace, private
keys, large files, merge conflicts) and `mise run dead` (a report, never a
gate). `go fix ./...` runs first inside `mise run build`, so the modernizers
apply before anything is compiled — which is how a corrupted `package` line was
caught this session.

What the source catalogue runs that this repo does not, and what each would
add:

| Tool | What it would find here |
|---|---|
| `gocritic` | `rangeValCopy` on large structs, `indexAlloc`, `hugeParam` |
| `unparam` | parameters and results nothing reads |
| `errcheck` | unchecked `cmd.Run()` in a build path — needs an excludes file for `fmt.Fprint*`, whose failures on a terminal cannot be handled |
| `gocyclo` / `gocognit` | the three functions over 60 lines |
| `dupl` | duplication — **run at threshold 100, not 50**; a declarative registry is identical on purpose |
| `gopls check` | inefficient string concatenation that other linters miss |

`go fix` rewrites your own code, including helpers written minutes earlier.
Re-read a region after running it before patching that region.

---

## Part 9 — The honest finding on generics

The source catalogue measured eight refactors on a comparable codebase and
reports that **every one increased the line count**. This repo measured the
same thing independently and agrees: existing code fell from 4354 to ~4120
lines across a full DRY pass, and one sweep — unifying sixteen line-splitting
sites behind `cli.Lines` — *added* ten lines while removing no logic at all.

Generics compress repeated logic that varies only by type. They do not compress
declarative data or orchestration, and Go's closure-in-struct-literal syntax
costs more than the straight-line code it replaces.

Where they paid here: `cli.DecodeJSON[T]`, `conf.Load[T]`, `cli.Map`/`Filter`/
`Count`/`CountBy`/`Pick`, and two generic **methods** — `tool.Result.JSON[T]`
and `Issues.By[K]`. Both of those are receiver-owned conversions, which is the
case Go's own blog and the Go 1.27 write-ups endorse; `Map`, `Filter` and
`Sorted` stay package functions, which those same sources explicitly warn
against converting.

The rule worth keeping: **an operation that constrains the receiver's own type
must be a free function**; a method may only add type parameters of its own.
`Seq[T any]` pins `T` to `any`, so a method cannot constrain it back.

**Judge a refactor by how many places a future change must touch, not by the
diff size** — and report the line count honestly either way.

---

## Part 10 — Method

How this was assembled, so it can be extended the same way.

1. **Search semantically.** Keyword search finds the obvious four. Semantic
   search is what surfaced kitsune, scry and teseo.
2. **Rank by importers, not stars.** A 230-star package was stale; a 48-star
   one had double the importers and recent commits.
3. **Install it and run it.** Three of nine binaries in the source catalogue
   could not be `go install`ed, one had no `go.mod`, one needed a daemon. None
   of that is in a README. Here: muffet needed three flags before it was usable
   and none of them is in its quickstart.
4. **Read the raw JSON before writing a parser.** Guessed key names and guessed
   types are where the silent bugs live — `average_seo_score` not
   `average_score`, `guideline_url` not `docs_url`, `og:image` as a nested
   array.
5. **Cross-check tools against each other.** Three checkers that disagree about
   one page is how you find out one of them is wrong.
6. **Read the release notes.** Go 1.27 has generic methods. An agent's training
   will tell you it does not.
