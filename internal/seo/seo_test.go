package seo

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
	"github.com/joeblew999/dev/internal/seo/checkers"
)

// recorded replaces the runner with one that hands back reports the checkers
// really printed, kept in testdata, keyed by the binary asked for. No crawler
// runs in a test: this tree skips nothing, so a test that needed a binary and
// a network would be a test that quietly stopped holding anything.
func recorded(t *testing.T, byTool map[string]string) {
	t.Helper()
	oldRun, oldTiming := tool.Run, tool.Timing
	tool.Timing = nil // a test says nothing about how long a fixture took
	tool.Run = func(bin string, args ...string) (tool.Result, error) {
		file, ok := byTool[bin]
		if !ok {
			return tool.Result{Bin: bin}, fmt.Errorf("%s is not on PATH; add it to mise.toml [tools]", bin)
		}
		data, err := os.ReadFile(filepath.Join("testdata", file))
		if err != nil {
			t.Fatal(err)
		}
		// A checker prints its progress first, and the real decode path skips
		// it: the fixture goes through everything the tool's output does.
		return tool.Result{Bin: bin, Out: "crawling…\n" + string(data), Took: 250 * time.Millisecond}, nil
	}
	t.Cleanup(func() { tool.Run, tool.Timing = oldRun, oldTiming })
}

// call is one invocation with a subcommand's flags parsed as cli parses them.
func call(t *testing.T, out io.Writer, flags func(*flag.FlagSet), args ...string) cli.Call {
	t.Helper()
	fs := cli.Flags("dev seo", io.Discard)
	flags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return cli.Call{Verb: "dev seo", Flags: fs, Stdout: out, Stderr: io.Discard}
}

func TestCheckReadsWhatEachCheckerFound(t *testing.T) {
	recorded(t, map[string]string{"scoutly": "scoutly-example.json"})
	c := call(t, io.Discard, CheckFlags)
	rep := cli.NewReport("seo", "https://example.com")
	pick, err := selection(c, names(checkers.All, func(ch checkers.Checker) string { return ch.Name }))
	if err != nil {
		t.Fatal(err)
	}
	Audit(c, rep, "https://example.com", 5, pick)
	rep.Done(time.Now(), cli.SevError)

	if len(rep.Steps) != len(checkers.All) {
		t.Fatalf("got %d steps; want one per checker (%d)", len(rep.Steps), len(checkers.All))
	}
	// Checkers run at once but merge in registry order, so a report is the
	// same bytes however they were scheduled.
	want := names(checkers.All, func(ch checkers.Checker) string { return ch.Name })
	for i, s := range rep.Steps {
		if s.Name != want[i] {
			t.Errorf("step %d is %q; want %q — the merge must keep registry order", i, s.Name, want[i])
		}
	}
	// A missing checker is recorded with the reason and what it would have
	// given; the others still run.
	for _, s := range rep.Steps {
		if s.Name == "scoutly" {
			continue
		}
		if s.Status != cli.StatusSkipped || s.Note == "" || s.Provides == "" {
			t.Errorf("%s = %+v; want it skipped, with why and what it gives", s.Name, s)
		}
	}
	if rep.BySeverity[cli.SevError] != 1 || rep.ByTool["scoutly"] != 2 {
		t.Errorf("counts = %v / %v; want one error, both findings from scoutly", rep.BySeverity, rep.ByTool)
	}
	if rep.Outcome != "fail" {
		t.Errorf("outcome = %q; want fail — an error must fail the gate", rep.Outcome)
	}
	for _, f := range rep.Findings {
		if f.Fix == "" || !strings.Contains(f.Fix, "https://developers.google.com/") {
			t.Errorf("%s carries no fix naming a Google page: %q", f.ID, f.Fix)
		}
	}
}

// --only and --skip name a checker that does not exist rather than silently
// running nothing.
func TestSelectionNamesATypo(t *testing.T) {
	c := call(t, io.Discard, CheckFlags, "--only", "scoutley")
	if _, err := selection(c, names(checkers.All, func(ch checkers.Checker) string { return ch.Name })); err == nil || !strings.Contains(err.Error(), `did you mean "scoutly"`) {
		t.Errorf("err = %v; want it to suggest scoutly", err)
	}
	c = call(t, io.Discard, CheckFlags, "--skip", "muffet")
	pick, err := selection(c, names(checkers.All, func(ch checkers.Checker) string { return ch.Name }))
	if err != nil {
		t.Fatal(err)
	}
	if pick.skipped("muffet") == "" || pick.skipped("scoutly") != "" {
		t.Errorf("skip = %q / %q; want muffet excluded and scoutly kept",
			pick.skipped("muffet"), pick.skipped("scoutly"))
	}
}

// What write writes, validate accepts — because both read one declaration. If
// they could disagree, an artifact would pass when written and fail when
// checked, which is the bug this shape exists to prevent.
func TestWriteThenValidateAgree(t *testing.T) {
	dir := t.TempDir()
	c := call(t, io.Discard, WriteFlags, "--quiet")
	site := Site{
		Origin: "https://example.com", URL: "https://example.com/page",
		// Long enough to clear the title range: validateHead reports a title
		// under 30 characters now, and a fixture that a validator faults is
		// one that cannot prove write and validate agree.
		Title: "A page worth finding, and what it answers",
		Desc:  "What this page answers, in one line.",
		Image: "https://example.com/og.png",
		URLs:  []string{"https://example.com/", "https://example.com/page"},
		Now:   time.Now().UTC(),
	}
	written := cli.NewReport("seo", dir)
	if err := Write(c, dir, site, written, nil); err != nil {
		t.Fatal(err)
	}
	for _, f := range written.Findings {
		t.Errorf("what was just written does not validate: %s %s — %s", f.Severity, f.ID, f.Message)
	}

	checked := cli.NewReport("seo", dir)
	if err := Validate(dir, site.Origin, checked, nil); err != nil {
		t.Fatal(err)
	}
	for _, f := range checked.Findings {
		t.Errorf("validate disagrees with write: %s %s — %s", f.Severity, f.ID, f.Message)
	}
	for _, name := range []string{"sitemap.xml", "robots.txt", "head.html"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was not written", name)
		}
	}
}

// The checks have to catch what they exist to catch.
func TestValidatorsCatchWhatTheyAreFor(t *testing.T) {
	for _, tc := range []struct {
		name, content, origin, want string
		check                       func(string, string, string) ([]cli.Finding, string)
	}{
		{"cross-host loc", `<urlset><url><loc>https://elsewhere.com/x</loc></url></urlset>`,
			"https://example.com", "sitemap-cross-host", validateSitemap},
		{"relative loc", `<urlset><url><loc>/about</loc></url></urlset>`,
			"https://example.com", "sitemap-relative-loc", validateSitemap},
		{"priority out of range", `<urlset><url><loc>https://example.com/</loc><priority>7</priority></url></urlset>`,
			"https://example.com", "sitemap-bad-priority", validateSitemap},
		{"empty sitemap", `<urlset></urlset>`, "https://example.com", "sitemap-empty", validateSitemap},
		{"robots blocks everything", "User-agent: *\nDisallow: /\n",
			"https://example.com", "robots-blocks-all", validateRobots},
		{"robots names no sitemap", "User-agent: *\nAllow: /\n",
			"https://example.com", "robots-no-sitemap", validateRobots},
		{"relative sitemap directive", "User-agent: *\nSitemap: /sitemap.xml\n",
			"https://example.com", "robots-relative-sitemap", validateRobots},
		{"head has no canonical", "<title>T</title>", "", "missing-canonical", validateHead},
		{"relative canonical", `<title>T</title><link rel="canonical" href="/page">`,
			"", "canonical-relative", validateHead},
		// absolute() was three different tests before it was one, and this was
		// the loose one: it asked only for the prefix "http", so a path that
		// begins with those four letters read as a full URL.
		{"canonical that only starts like a URL",
			`<title>T</title><link rel="canonical" href="httpsites/page">`,
			"", "canonical-relative", validateHead},

		// The live checkers found these eight on this repo's own deployed
		// site. Each one below is a validator that could have said so before
		// the deploy and did not.
		//
		// seo-audit's TITLE_LENGTH, which is its name for both ends of the
		// range: the site's title was "dev" and only the long end was read.
		{"title of one word", "<title>dev</title>", "", "title-too-short", validateHead},
		// kitsune's seo.open_graph.incomplete names whichever og tag is
		// absent, and two of the five writeHead emits were never read back.
		{"open graph without og:url",
			`<title>A title long enough to describe the page</title><meta property="og:type" content="website">`,
			"", "missing-og-url", validateHead},
		{"open graph without og:type",
			`<title>A title long enough to describe the page</title><meta property="og:url" content="https://example.com/">`,
			"", "missing-og-type", validateHead},
		// kitsune's security.csp.unsafe.style-src and scry's
		// security/csp-unsafe, both against a _headers this package wrote and
		// then validated as good, because it only asked whether the header
		// was there.
		{"csp allows inline style",
			"/*\n  Content-Security-Policy: default-src 'self'; style-src 'self' 'unsafe-inline'\n",
			"", "headers-csp-unsafe-inline", validateHeaders},
		{"csp allows eval",
			"/*\n  Content-Security-Policy: default-src 'self'; script-src 'unsafe-eval'\n",
			"", "headers-csp-unsafe-eval", validateHeaders},
		{"csp allows any source",
			"/*\n  Content-Security-Policy: default-src 'self'; img-src *\n",
			"", "headers-csp-wildcard", validateHeaders},
		{"csp with no floor",
			"/*\n  Content-Security-Policy: script-src 'self'\n",
			"", "headers-csp-no-default-src", validateHeaders},
		// A header that is present and switched off reads as done to every
		// checker that only counts headers, which is what this used to be.
		{"hsts expires immediately",
			"/*\n  Strict-Transport-Security: max-age=0\n",
			"", "headers-hsts-disabled", validateHeaders},
		{"nosniff misspelled",
			"/*\n  X-Content-Type-Options: none\n",
			"", "headers-nosniff-wrong", validateHeaders},
		// scry's health/missing-charset, in the half that is in a file: the
		// live one is the type the host serves and no file here holds it.
		{"content type without an encoding",
			"/*\n  Content-Type: text/html\n",
			"", "headers-content-type-no-charset", validateHeaders},
		// A header named only in a comment used to count as present, because
		// the check was a substring search over the whole file.
		{"header only mentioned in a comment",
			"/*\n  # X-Frame-Options: SAMEORIGIN\n",
			"", "missing-header-x-frame-options", validateHeaders},
	} {
		found, covered := tc.check(tc.name, tc.content, tc.origin)
		if covered == "" {
			t.Errorf("%s: the check says nothing about what it looked at", tc.name)
		}
		ids := cli.Map(found, func(f cli.Finding) string { return f.ID })
		if !cli.ToSet(ids)[tc.want] {
			t.Errorf("%s: got %v; want %s", tc.name, ids, tc.want)
		}
		for _, f := range found {
			if f.Fix == "" {
				t.Errorf("%s: %s carries no fix", tc.name, f.ID)
			}
			// An id no writer claims is a finding a reader can do nothing
			// with: `check --fix` routes by prefix-matching Writer.Fixes, so
			// a validator that emits an unclaimed id is a dead end.
			if fixedBy(f.ID) == "" {
				t.Errorf("%s: %s names no writer that fixes it — add the prefix to one, "+
					"or the report is a fault with no route out", tc.name, f.ID)
			}
		}
	}
}

// A CSP that names its sources and allows nothing unsafe is left alone.
//
// The line validateHeaders draws is correctness, not preference: it reports a
// value that cannot do the header's job and never one that is merely not the
// value this tool would have picked. A check that faulted a good policy would
// be the second kind, and nobody would keep it on.
func TestHeaderPolicyLeavesAGoodValueAlone(t *testing.T) {
	// Through writeHeaders and not a file spelled out here: a second copy of
	// the header table drifts the day the table gains a header, which it did
	// while this test was being written.
	good, _, err := writeHeaders(Site{CSP: "default-src 'self'; script-src 'none'; img-src 'self' data:"})
	if err != nil {
		t.Fatal(err)
	}
	found, covered := validateHeaders("_headers", good, "")
	for _, f := range found {
		t.Errorf("a sound _headers was faulted: %s %s — %s", f.Severity, f.ID, f.Message)
	}
	if covered == "" {
		t.Error("the check says nothing about what it looked at")
	}
}

// And the same file with the policy this repo's own site is served with is
// not left alone. That is the bug in one line: this package wrote that CSP
// into _headers, validated the file it had just written and called it good,
// while kitsune and scry each reported the 'unsafe-inline' in it against the
// deployed site.
func TestTheCSPThisSiteShipsIsCaughtBeforeItDeploys(t *testing.T) {
	content, _, err := writeHeaders(Site{CSP: "default-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"script-src 'none'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"})
	if err != nil {
		t.Fatal(err)
	}
	found, _ := validateHeaders("_headers", content, "")
	ids := cli.Map(found, func(f cli.Finding) string { return f.ID })
	if !cli.ToSet(ids)["headers-csp-unsafe-inline"] {
		t.Errorf("got %v; want headers-csp-unsafe-inline — a deploy should not be "+
			"the first thing that says so", ids)
	}
}

// A finding that stops Google indexing the site is an error; one that makes a
// preview or a description worse is a warning, because --fail-on error is
// what a repo puts in CI and that line decides whether a build fails.
//
// robots-relative-sitemap was the error that should not have been: Google
// ignores a relative Sitemap: line, which leaves the file naming no sitemap —
// exactly the state robots-no-sitemap reports as a warning. One outcome
// cannot fail a build two ways depending on how it was spelled.
func TestSeveritiesFollowTheConsequence(t *testing.T) {
	for _, tc := range []struct {
		name, content, id, want string
		check                   func(string, string, string) ([]cli.Finding, string)
	}{
		{"nothing is indexed", "User-agent: *\nDisallow: /\n",
			"robots-blocks-all", cli.SevError, validateRobots},
		{"the sitemap is not found", "User-agent: *\nSitemap: /sitemap.xml\n",
			"robots-relative-sitemap", cli.SevWarning, validateRobots},
		{"the sitemap is not named", "User-agent: *\nAllow: /\n",
			"robots-no-sitemap", cli.SevWarning, validateRobots},
		{"the result has no headline", "<meta name=\"description\" content=\"x\">",
			"missing-title", cli.SevError, validateHead},
		{"the headline is thin", "<title>dev</title>",
			"title-too-short", cli.SevWarning, validateHead},
		{"the preview is poorer", "<title>A title long enough to describe the page</title>",
			"missing-og-image", cli.SevWarning, validateHead},
		{"none of the headers applies", "  X-Frame-Options: SAMEORIGIN\n",
			"headers-no-rule", cli.SevError, validateHeaders},
		{"a header is present and off", "/*\n  Strict-Transport-Security: max-age=0\n",
			"headers-hsts-disabled", cli.SevWarning, validateHeaders},
	} {
		found, _ := tc.check(tc.name, tc.content, "https://example.com")
		got := ""
		for _, f := range found {
			if f.ID == tc.id {
				got = f.Severity
			}
		}
		switch {
		case got == "":
			t.Errorf("%s: %s was not reported at all", tc.name, tc.id)
		case got != tc.want:
			t.Errorf("%s: %s is a %s; want %s — %s", tc.name, tc.id, got, tc.want, tc.name)
		}
	}
}

// directives reads the one format robots.txt and _headers share, and both
// validators had written the walk out. What it must get right is the two
// things that are not directives: a comment, and a line with no colon — which
// is what a path rule in _headers is.
func TestDirectivesReadsTheFormatBothFilesShare(t *testing.T) {
	got := directives("# a comment\n\n/*\n  X-Frame-Options: SAMEORIGIN\nSitemap: https://example.com/sitemap.xml\n")
	want := []directive{
		{"x-frame-options", "SAMEORIGIN"},
		{"sitemap", "https://example.com/sitemap.xml"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %v; want %v", i, got[i], want[i])
		}
	}
}

// The verb end to end: JSON on stdout, the report written where --out says,
// each checker's own output kept beside it, and a non-zero result because the
// page has something to fix.
func TestCheckAnswersJSONAndGates(t *testing.T) {
	recorded(t, map[string]string{"scoutly": "scoutly-example.json"})
	dir := t.TempDir()
	path := filepath.Join(dir, "seo.json")

	var out bytes.Buffer
	c := call(t, &out, CheckFlags, "--json", "--out", path, "--quiet")
	c.Args = []string{"https://example.com"}
	if err := runCheck(c); err == nil {
		t.Error("runCheck returned nil; a page with an error must fail the gate")
	}

	var rep cli.Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if rep.Outcome != "fail" || rep.Target != "https://example.com" {
		t.Errorf("report = %s on %s; want a failing audit of example.com", rep.Outcome, rep.Target)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, out.Bytes()) {
		t.Error("the file and stdout differ; they are meant to be the same bytes")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "seo", "scoutly.json"))
	if err != nil {
		t.Fatalf("scoutly's own output was not kept: %v", err)
	}
	if !strings.Contains(string(raw), "missing-meta-description") {
		t.Error("the sub-report does not hold what the checker printed")
	}
}

// A code this package has no advice for still reports, with the tool's own
// message and the documentation index, rather than being dropped.
func TestAnUnknownCodeStillCarriesSomewhereToLook(t *testing.T) {
	if fix := checkers.Fix("something-scoutly-added-later"); !strings.Contains(fix, checkers.DocEssentials) {
		t.Errorf("fix = %q; want it to point at the essentials", fix)
	}
}

// A report is only useful if it says how to act on it. Every finding a writer
// can fix names that writer, and one no file can fix names nothing — a broken
// link is content, not a tag.
func TestFindingsNameTheWriterThatFixesThem(t *testing.T) {
	for id, want := range map[string]string{
		"missing-canonical":           "head",
		"canonical-relative":          "head",
		"missing-meta-description":    "head",
		"missing-og-image":            "head",
		"seo.canonical.missing":       "head", // kitsune's dotted id, same writer
		"seo.description.short":       "head",
		"geo.jsonld.absent":           "head",
		"sitemap-cross-host":          "sitemap",
		"seo.sitemap.bad_status":      "sitemap",
		"robots-blocks-all":           "robots",
		"seo.robots_txt.missing":      "robots",
		"broken-link":                 "", // content, and sometimes someone else's
		"unreachable-link":            "",
		"perf.render_blocking.styles": "",
	} {
		if got := fixedBy(id); got != want {
			t.Errorf("fixedBy(%q) = %q; want %q", id, got, want)
		}
	}
}

// Every id a writer claims to fix must be one its own validator can actually
// report, or the route it promises is a dead end.
func TestWritersOnlyClaimWhatTheyCanFix(t *testing.T) {
	for _, w := range writers {
		for _, prefix := range w.Fixes {
			if fixedBy(prefix) != w.Name {
				t.Errorf("%s claims %q but fixedBy sends it to %q", w.Name, prefix, fixedBy(prefix))
			}
		}
	}
}

// The catalogue is the inventory a person reads before touching this, and
// prose drifts: the headers writer existed for a day before the document
// knew. Nothing can check that what it says is true, but it can be held to
// naming everything that exists — which is the drift that actually happened.
func TestCatalogueNamesEveryToolAndWriter(t *testing.T) {
	doc, err := os.ReadFile("catalogue.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range append(checkerNames(), writerNames()...) {
		if !strings.Contains(string(doc), name) {
			t.Errorf("catalogue.md does not name %q; a tool nobody wrote down is one nobody knows is there", name)
		}
	}
	// And every file a writer produces, since that is what a reader looks for.
	for _, w := range writers {
		if !strings.Contains(string(doc), w.Produces.Name) {
			t.Errorf("catalogue.md does not name %q, which %s writes", w.Produces.Name, w.Name)
		}
	}
}
