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
	tool.Run = func(bin, pin string, args ...string) (tool.Result, error) {
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
		Title: "A page worth finding", Desc: "What this page answers, in one line.",
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
