// The checks that say an artifact is good. One implementation each, used by
// `write` on what it just wrote and by `validate` on what is on disk — so an
// artifact cannot pass at write time and fail at check time.
//
// Every finding names its fix and the Google page that explains why, the same
// rule the live checkers follow.
package seo

import (
	"encoding/xml"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/seo/checkers"
)

// validateSitemap holds a sitemap to sitemaps.org: absolute locations on the
// site's own host, a priority in range, and the caps Google enforces.
func validateSitemap(name, content, origin string) (found []cli.Finding, covered string) {
	var doc struct {
		XMLName xml.Name `xml:"urlset"`
		URLs    []struct {
			Loc      string `xml:"loc"`
			LastMod  string `xml:"lastmod"`
			Priority string `xml:"priority"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal([]byte(content), &doc); err != nil {
		return one("sitemap-unparsable", cli.SevError,
			"sitemap.xml is not valid XML: "+err.Error(),
			"write it with: dev seo write <dir> — "+docSitemaps), "unreadable"
	}
	var out []cli.Finding
	if len(doc.URLs) == 0 {
		out = append(out, finding("sitemap-empty", cli.SevError,
			"sitemap.xml lists no URLs",
			"a sitemap with nothing in it tells Google nothing — "+docSitemaps))
	}
	// Google stops reading at 50,000 URLs or 50MB, whichever comes first, and
	// says nothing about the rest.
	if n := len(doc.URLs); n > 50000 {
		out = append(out, finding("sitemap-too-many", cli.SevError,
			fmt.Sprintf("%d URLs; Google reads 50,000 per file", n),
			"split it and list the parts in a sitemap index — "+docSitemaps))
	}
	if len(content) > 50<<20 {
		out = append(out, finding("sitemap-too-large", cli.SevError,
			"over 50MB uncompressed; Google reads no further",
			"split it into several files and index them — "+docSitemaps))
	}
	for _, u := range doc.URLs {
		switch {
		case u.Loc == "":
			out = append(out, finding("sitemap-empty-loc", cli.SevError,
				"a <url> has no <loc>", "every entry needs an absolute URL — "+docSitemaps))
		case !absolute(u.Loc):
			out = append(out, findingAt("sitemap-relative-loc", cli.SevError, u.Loc,
				"<loc> is not absolute: "+u.Loc,
				"write the full URL, scheme and host included — "+docSitemaps))
		case origin != "" && originOf(u.Loc) != origin:
			out = append(out, findingAt("sitemap-cross-host", cli.SevError, u.Loc,
				fmt.Sprintf("<loc> is on another host: %s, not %s", originOf(u.Loc), origin),
				"a sitemap may only list URLs on its own host — "+docSitemaps))
		}
		if u.Priority != "" {
			if p, err := strconv.ParseFloat(u.Priority, 64); err != nil || p < 0 || p > 1 {
				out = append(out, findingAt("sitemap-bad-priority", cli.SevWarning, u.Loc,
					"priority "+u.Priority+" is outside 0.0–1.0",
					"use a number between 0.0 and 1.0, or leave it out — "+docSitemaps))
			}
		}
	}
	return out, cli.Plural(len(doc.URLs), "URL")
}

// validateRobots holds robots.txt to what Google reads: it must say something,
// it must not block everything by accident, and it should name the sitemap.
func validateRobots(name, content, origin string) (found []cli.Finding, covered string) {
	var out []cli.Finding
	lines := directives(content)
	sitemaps := 0
	blocksAll := false
	agent := ""
	for _, d := range lines {
		switch d.key {
		case "user-agent":
			agent = d.value
		case "disallow":
			if d.value == "/" && (agent == "*" || strings.EqualFold(agent, "googlebot")) {
				blocksAll = true
			}
		case "sitemap":
			sitemaps++
			if !absolute(d.value) {
				// A warning and not an error, because the consequence is the
				// one robots-no-sitemap already reports at that severity:
				// Google ignores a relative Sitemap: line, so the file names
				// no sitemap. Two spellings of one outcome cannot fail a
				// build differently.
				out = append(out, finding("robots-relative-sitemap", cli.SevWarning,
					"Sitemap: must be an absolute URL, not "+d.value,
					"write the full URL, scheme and host included — "+checkers.DocRobotsIntro))
			}
		}
	}
	if len(lines) == 0 {
		out = append(out, finding("robots-empty", cli.SevWarning,
			"robots.txt holds no directives",
			"an empty file allows everything, which is fine — but say so, and name the sitemap — "+checkers.DocRobotsIntro))
	}
	if blocksAll {
		out = append(out, finding("robots-blocks-all", cli.SevError,
			"robots.txt disallows everything for Googlebot",
			"remove the Disallow: / — Google will not index what it cannot fetch — "+checkers.DocRobotsIntro))
	}
	if sitemaps == 0 {
		out = append(out, finding("robots-no-sitemap", cli.SevWarning,
			"robots.txt names no Sitemap:",
			"add Sitemap: "+cli.Or(origin, "https://example.com")+"/sitemap.xml — it is how Google finds the list — "+docSitemaps))
	}
	// Google reads the first 500 KiB and ignores the rest.
	if len(content) > 500<<10 {
		out = append(out, finding("robots-too-large", cli.SevError,
			"over 500 KiB; Google reads no further",
			"keep it small — what is past the limit is not applied — "+checkers.DocRobotsIntro))
	}
	covered = cli.Plural(len(lines), "directive")
	if sitemaps > 0 {
		covered += ", sitemap named"
	}
	return out, covered
}

// validateHead holds a head fragment to what Search and a link preview read.
//
// What writeHead is answerable for, and nothing else. A head fragment is not
// the whole of a page's head: `charset` and `viewport` live in the page
// template, which this command does not write, so their absence *here* says
// nothing about the page and is not reported. Every tag below is one
// writeHead emits, which is what makes each finding closeable by re-running
// the writer.
//
// One writeHead emits is missing from the list: the `rel="icon"` and
// `rel="apple-touch-icon"` links. Reading those back wants a finding id the
// head writer claims, and its Fixes has no prefix that matches one — so the
// check waits on that line rather than emitting an id `check --fix` would
// drop on the floor.
func validateHead(name, content, origin string) (found []cli.Finding, covered string) {
	var out []cli.Finding
	title := between(content, "<title>", "</title>")
	// Characters, not bytes: Search truncates on what it renders, and a title
	// of sixty accented letters is sixty characters and a hundred and twenty
	// bytes.
	length := utf8.RuneCountInString(title)
	switch {
	case title == "":
		out = append(out, finding("missing-title", cli.SevError,
			"no <title>", checkers.Fix("missing-title")))
	case length > 60:
		out = append(out, finding("title-too-long", cli.SevWarning,
			fmt.Sprintf("<title> is %d characters; Search truncates near 60", length),
			checkers.Fix("title-too-long")))
	case length < 30:
		// seo-audit reported TITLE_LENGTH against this repo's own site and
		// nothing here could have said so first: the title was "dev", three
		// characters, and this function only looked at the long end of the
		// range. A one-word title describes nothing for a query to match.
		out = append(out, finding("title-too-short", cli.SevWarning,
			fmt.Sprintf("<title> is %d characters; Search has little to match a query against", length),
			checkers.Fix("title-too-short")))
	}
	if strings.Count(content, "<title>") > 1 {
		out = append(out, finding("duplicate-title", cli.SevError,
			"more than one <title>", checkers.Fix("duplicate-title")))
	}
	present := 0
	for _, want := range []struct{ marker, code string }{
		{`name="description"`, "missing-meta-description"},
		{`rel="canonical"`, "missing-canonical"},
		{`property="og:title"`, "missing-og-title"},
		{`property="og:description"`, "missing-og-description"},
		// og:url and og:type are written by writeHead and were not read back,
		// so kitsune's seo.open_graph.incomplete could name either of them on
		// a deployed page while this said the fragment was whole.
		{`property="og:url"`, "missing-og-url"},
		{`property="og:type"`, "missing-og-type"},
		{`property="og:image"`, "missing-og-image"},
		{`application/ld+json`, "missing-json-ld"},
	} {
		if strings.Contains(content, want.marker) {
			present++
			continue
		}
		out = append(out, finding(want.code, cli.SevWarning,
			"no "+want.marker, checkers.Fix(want.code)))
	}
	// A canonical that is not absolute resolves against whatever page includes
	// the fragment, so one file names a different URL on every page it is on —
	// which is the tag saying something other than what was meant.
	if href := between(content, `rel="canonical" href="`, `"`); href != "" && !absolute(href) {
		out = append(out, finding("canonical-relative", cli.SevError,
			"canonical is not an absolute URL: "+href,
			"write the full URL, scheme and host included — "+checkers.DocCanonical))
	}
	if title != "" {
		present++
	}
	return out, fmt.Sprintf("%d of 9 tags Search reads", present)
}

// validateHeaders reads a _headers file back: every header this writes is
// there, a path rule applies them, and the values that can be wrong are not.
//
// Presence and policy, and the line between them has moved. It used to check
// presence only, on the reasoning that whether a value is right for a site is
// that site's business. That is still true of every value with more than one
// right answer — how long HSTS should last, which referrer policy a site
// wants, what a Content-Security-Policy may load — and none of those is
// checked here. It was never true of a value that cannot do the header's job
// whatever the site is: `'unsafe-inline'` in a policy whose purpose is to
// stop inline injection, an HSTS that expires immediately, an
// X-Content-Type-Options a browser will ignore. Two checkers reported the
// first of those against this repo's own site while this function read the
// same file and called it clean.
//
// So the line is correctness, not preference: a value is reported when it
// makes the header a no-op, and never when it is merely not the value this
// tool would have chosen.
func validateHeaders(name, content, origin string) (found []cli.Finding, covered string) {
	set := map[string]string{}
	for _, d := range directives(content) {
		set[d.key] = d.value
	}
	present := 0
	for _, h := range headers {
		if _, ok := set[strings.ToLower(h.name)]; ok {
			present++
			continue
		}
		found = append(found, finding("missing-header-"+strings.ToLower(h.name),
			cli.SevWarning, "no "+h.name,
			h.why+" — set it in "+name+" — "+checkers.DocEssentials))
	}
	if !strings.Contains(content, "/*") {
		found = append(found, finding("headers-no-rule", cli.SevError,
			"no path rule, so none of these headers is applied to anything",
			"the file needs a path line such as /* before its headers — "+checkers.DocEssentials))
	}
	for _, p := range policies {
		if value, ok := set[p.header]; ok {
			found = append(found, p.faults(value)...)
		}
	}
	return found, cli.Plural(present, "header") + " of " + strconv.Itoa(len(headers))
}

// policies are the headers whose value can be present and still not work, and
// what is wrong with one when it is. A registry, so a header joins the policy
// half by being an entry here — and a header whose value is the site's own
// choice has no entry at all, which is how the line above is kept.
//
// Keyed as directives spells a name: lowercased.
var policies = []struct {
	header string
	faults func(value string) []cli.Finding
}{
	{"content-security-policy", cspFaults},
	{"strict-transport-security", hstsFaults},
	{"x-content-type-options", nosniffFaults},
	{"content-type", charsetFaults},
}

// cspFaults reads a Content-Security-Policy for the sources that make it a
// header that is there and stops nothing.
//
// kitsune reported security.csp.unsafe.style-src and scry reported
// security/csp-unsafe against this repo's own site, independently, while this
// package wrote that CSP into _headers and validated the file as good. A
// policy that allows inline style or script does not stop the injection the
// header exists to stop, and a source list of `*` is the same as no policy
// for that resource — neither is a matter of taste.
func cspFaults(value string) []cli.Finding {
	var out []cli.Finding
	named := map[string]bool{}
	for part := range strings.SplitSeq(value, ";") {
		name, sources, _ := strings.Cut(strings.TrimSpace(part), " ")
		name = strings.ToLower(name)
		if name == "" {
			continue
		}
		named[name] = true
		for _, unsafe := range []string{"unsafe-inline", "unsafe-eval"} {
			if strings.Contains(sources, "'"+unsafe+"'") {
				out = append(out, findingAt("headers-csp-"+unsafe, cli.SevWarning, name,
					name+" allows '"+unsafe+"'", checkers.Fix("csp-unsafe")))
			}
		}
		if slices.Contains(strings.Fields(sources), "*") {
			out = append(out, findingAt("headers-csp-wildcard", cli.SevWarning, name,
				name+" allows any source",
				"name the origins this site loads "+name+" from; * is the same as "+
					"having no policy for it — "+checkers.DocEssentials))
		}
	}
	if !named["default-src"] {
		out = append(out, finding("headers-csp-no-default-src", cli.SevWarning,
			"the policy sets no default-src, so anything it does not name is unrestricted",
			"add default-src 'self' as the floor, then loosen only the directives "+
				"that need it — "+checkers.DocEssentials))
	}
	return out
}

// hstsFaults: max-age is what the header is. Without one it is ignored, and
// at zero it is how HSTS is switched off — either way a header that is
// present and doing nothing, which is worse than one that is absent because
// every checker reads it as done.
func hstsFaults(value string) []cli.Finding {
	for part := range strings.SplitSeq(value, ";") {
		key, age, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "max-age") {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(age)); err == nil && n > 0 {
			return nil
		}
	}
	return one("headers-hsts-disabled", cli.SevWarning,
		"Strict-Transport-Security has no max-age above zero, so it does nothing",
		"set max-age=31536000; includeSubDomains, or the first request is still "+
			"plain http — "+checkers.DocEssentials)
}

// nosniffFaults: nosniff is the only value a browser acts on, so any other
// spelling is a header that is present and ignored.
func nosniffFaults(value string) []cli.Finding {
	if strings.EqualFold(value, "nosniff") {
		return nil
	}
	return one("headers-nosniff-wrong", cli.SevWarning,
		"X-Content-Type-Options is "+strconv.Quote(value)+", which no browser acts on",
		"set it to exactly nosniff, or a browser may still guess a type and run "+
			"a .txt as a script — "+checkers.DocEssentials)
}

// charsetFaults: a Content-Type this file declares and leaves without a
// charset makes the browser guess the encoding, which is the fault scry
// reports as health/missing-charset.
//
// Only when the file declares one, which is the half a validator can see.
// The live finding was about the type the edge served for an extensionless
// URL, and the answer to that was to write the header rather than to read it
// — see the note on what a validator can and cannot say in catalogue.md.
func charsetFaults(value string) []cli.Finding {
	if strings.Contains(strings.ToLower(value), "charset=") {
		return nil
	}
	return one("headers-content-type-no-charset", cli.SevWarning,
		"Content-Type is "+value+" with no charset, so the browser guesses the encoding",
		checkers.Fix("missing-charset"))
}

// absolute says whether a URL is one a crawler can follow from anywhere:
// scheme and host included.
//
// Three validators asked this and three spelled it differently, and one of
// the three tested only for the prefix "http" — which passes "httpx", and
// passes a path that happens to begin with those four letters.
func absolute(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// directive is one `Name: value` line of a line-oriented configuration file.
type directive struct{ key, value string }

// directives reads the format robots.txt and _headers are both written in:
// one `Name: value` per line, `#` for a comment, blank lines ignored. The key
// comes back lowercased because neither format cares about case and a caller
// comparing against "sitemap" should not have to know that.
//
// Both validators had written this walk out, and the second one needed the
// values rather than a substring search: a header can be present and still be
// wrong, which is not a question `strings.Contains` can be asked.
//
// A line with no colon is not a directive and is not counted as one. That is
// what a path rule in _headers is, and what a typo in robots.txt is.
func directives(content string) []directive {
	var out []directive
	for _, line := range cli.Lines(content) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out = append(out, directive{
			key:   strings.ToLower(strings.TrimSpace(key)),
			value: strings.TrimSpace(value),
		})
	}
	return out
}

func between(s, open, close string) string {
	_, after, ok := strings.Cut(s, open)
	if !ok {
		return ""
	}
	before, _, ok := strings.Cut(after, close)
	if !ok {
		return ""
	}
	return strings.TrimSpace(before)
}

func finding(code, sev, msg, fix string) cli.Finding {
	return cli.Finding{Severity: sev, ID: code, Message: msg, Fix: fix}
}

func findingAt(code, sev, where, msg, fix string) cli.Finding {
	f := finding(code, sev, msg, fix)
	f.Where = where
	return f
}

func one(code, sev, msg, fix string) []cli.Finding {
	return []cli.Finding{finding(code, sev, msg, fix)}
}

const docSitemaps = "https://developers.google.com/search/docs/crawling-indexing/sitemaps/overview"

// validateLlms holds llms.txt to the convention it is written against: an H1
// naming the site, and at least one link under it.
//
// Not a strict parser, deliberately. The file is markdown a person may edit
// by hand after this writes it, and a validator that refuses their wording is
// one they will delete. What it catches is the two ways the file is useless:
// no heading, so nothing says what the site is, and no links, so nothing says
// where anything is.
func validateLlms(name, content, origin string) (found []cli.Finding, covered string) {
	var out []cli.Finding
	var title, summary string
	links := 0
	for _, line := range cli.Lines(content) {
		trimmed := strings.TrimSpace(line)
		switch {
		case title == "" && strings.HasPrefix(trimmed, "# "):
			title = strings.TrimSpace(trimmed[2:])
		case summary == "" && strings.HasPrefix(trimmed, "> "):
			summary = strings.TrimSpace(trimmed[2:])
		case strings.Contains(trimmed, "](") && strings.HasPrefix(trimmed, "-"):
			links++
		}
	}
	// Through the same two builders every other validator uses. Written out
	// as literals these also set Tool, which each() overwrites with the
	// writer's name on every finding it collects — a fact spelled twice, and
	// the second spelling was the one that never applied.
	if title == "" {
		out = append(out, findingAt("llms-no-title", cli.SevError, name,
			name+" has no H1, so nothing names the site",
			"start the file with `# <the site's name>`"))
	}
	if summary == "" {
		out = append(out, findingAt("llms-no-summary", cli.SevWarning, name,
			name+" has no one-line summary",
			"a `> ` blockquote under the title is what a model quotes when it describes you"))
	}
	if links == 0 {
		out = append(out, findingAt("llms-no-links", cli.SevError, name,
			name+" lists no pages",
			"list them as `- [name](url)` under an `## ` heading"))
	}
	named := "untitled"
	if title != "" {
		named = strconv.Quote(title)
	}
	return out, cli.Plural(links, "link") + " under " + named
}
