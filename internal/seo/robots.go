// icanhasrobot is Google's own robots.txt matcher, as a binary.
//
// It ships in jimsmart/grobotstxt, which is a native Go port of Google's C++
// robots.txt parser, preserving its behaviour and its test suite. That is the
// difference worth paying for: every other robots library here, temoto's
// included, is someone's reading of the spec, and they disagree at exactly
// the edges that matter — longest-match precedence, `$` anchoring, Allow
// beating Disallow. Google's matcher is not an opinion about the rules; it is
// the rules.
//
// Shelled rather than imported, so the authority costs no dependency: this
// tool has two, and the port would be a third.
package seo

import (
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
)

var robotsRules = Checker{
	Name:     "icanhasrobot",
	Pin:      `"go:github.com/jimsmart/grobotstxt/cmd/icanhasrobot" = "latest"`,
	Provides: "Google's own matcher on whether Googlebot may fetch this exact URL",
	Cost:     "<10ms",
	// It takes the rules as a file rather than a URL, so they are fetched
	// first — as Googlebot, because what matters is what Google is served.
	Fetch: "/robots.txt",
	Args: func(a Ask) []string {
		return []string{a.File, "Googlebot", a.URL}
	},
	Read: readRobotsRules,
}

// readRobotsRules reads its prose. The verdict is the end of a line rather
// than the end of the output: with no rules at all it prints ALLOWED and then
// a notice saying why, and matching the last line called that no verdict.
// Match by pattern, never by position.
func readRobotsRules(res tool.Result) (Found, error) {
	found := Found{Summary: "1 URL against the live rules"}
	verdict := ""
	empty := false
	for _, line := range cli.Lines(res.Out) {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasSuffix(line, "DISALLOWED"):
			verdict = "DISALLOWED"
		case strings.HasSuffix(line, "ALLOWED"):
			verdict = "ALLOWED"
		case strings.Contains(line, "robots file is empty"):
			empty = true
		}
	}
	switch {
	case verdict == "DISALLOWED":
		found.Issues = append(found.Issues, cli.Finding{
			Tool:     "icanhasrobot",
			ID:       "blocked-by-robots",
			Severity: cli.SevError,
			Message:  "robots.txt blocks Googlebot from this URL, by Google's own matcher",
			Fix: "remove the rule that matches it, or accept that Google will not " +
				"index this page — " + docRobotsIntro,
		})
	case empty:
		// Allow-all, which is fine in itself — but a site with no rules has
		// no Sitemap: line either, and that is how Google finds the list.
		found.Summary = "no robots.txt: everything allowed"
		found.Issues = append(found.Issues, cli.Finding{
			Tool:     "icanhasrobot",
			ID:       "robots-missing",
			Severity: cli.SevWarning,
			Message:  "the site serves no robots.txt, so it names no Sitemap: either",
			Fix: "write one with: dev seo write <dir> — Google reads a missing file " +
				"as allow-all, but finds no sitemap from it — " + docRobotsIntro,
		})
	case verdict == "":
		found.Summary = "no verdict"
	}
	return found, nil
}
