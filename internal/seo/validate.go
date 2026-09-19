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
	"strconv"
	"strings"

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
		case !strings.HasPrefix(u.Loc, "http://") && !strings.HasPrefix(u.Loc, "https://"):
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
	var directives, sitemaps int
	blocksAll := false
	agent := ""
	for _, line := range cli.Lines(content) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		directives++
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		switch key {
		case "user-agent":
			agent = value
		case "disallow":
			if value == "/" && (agent == "*" || strings.EqualFold(agent, "googlebot")) {
				blocksAll = true
			}
		case "sitemap":
			sitemaps++
			if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
				out = append(out, finding("robots-relative-sitemap", cli.SevError,
					"Sitemap: must be an absolute URL, not "+value,
					"write the full URL, scheme and host included — "+checkers.DocRobotsIntro))
			}
		}
	}
	if directives == 0 {
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
	covered = cli.Plural(directives, "directive")
	if sitemaps > 0 {
		covered += ", sitemap named"
	}
	return out, covered
}

// validateHead holds a head fragment to what Search and a link preview read.
// Presence, not taste: a missing canonical is a fact, a bad title is not.
func validateHead(name, content, origin string) (found []cli.Finding, covered string) {
	var out []cli.Finding
	title := between(content, "<title>", "</title>")
	switch {
	case title == "":
		out = append(out, finding("missing-title", cli.SevError,
			"no <title>", checkers.Fix("missing-title")))
	case len(title) > 60:
		out = append(out, finding("title-too-long", cli.SevWarning,
			fmt.Sprintf("<title> is %d characters; Search truncates near 60", len(title)),
			checkers.Fix("title-too-long")))
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
	// A canonical that is not absolute is the one that silently does nothing.
	if href := between(content, `rel="canonical" href="`, `"`); href != "" &&
		!strings.HasPrefix(href, "http") {
		out = append(out, finding("canonical-relative", cli.SevError,
			"canonical is not an absolute URL: "+href,
			"write the full URL, scheme and host included — "+checkers.DocCanonical))
	}
	if title != "" {
		present++
	}
	return out, fmt.Sprintf("%d of 7 tags Search reads", present)
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
