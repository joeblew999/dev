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
package checkers

import (
	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
)

var robotsRules = Checker{
	Name:     "icanhasrobot",
	Spec:     "go:github.com/jimsmart/grobotstxt/cmd/icanhasrobot@latest",
	Provides: "Google's own matcher on whether Googlebot may fetch this exact URL",
	Cost:     "<10ms",
	// It takes the rules as a file rather than a URL, so they are fetched
	// first — as Googlebot, because what matters is what Google is served.
	Fetch: "/robots.txt",
	Args: func(a Ask) []string {
		return []string{a.File, "Googlebot", a.URL}
	},
	Read: fromRobots(),
}

// The prose it prints: a verdict at the end of a line, and with no rules at
// all a notice after it saying why. Match by pattern, never by position —
// reading the last line called that no verdict.
func fromRobots() func(tool.Result) (Found, error) {
	read := Text(
		Rule{When: Ends("DISALLOWED"), Then: func(_ string, f *Found) {
			f.Issues = append(f.Issues, Issue("icanhasrobot", "blocked-by-robots", cli.SevError,
				"robots.txt blocks Googlebot from this URL, by Google's own matcher", "",
				"remove the rule that matches it, or accept that Google will not index "+
					"this page — "+DocRobotsIntro))
		}},
		// Allow-all, which is fine in itself — but a site with no rules names
		// no Sitemap: either, and that is how Google finds the list.
		Rule{When: Has("robots file is empty"), Then: func(_ string, f *Found) {
			f.Summary = "no robots.txt: everything allowed"
			f.Issues = append(f.Issues, Issue("icanhasrobot", "robots-missing", cli.SevWarning,
				"the site serves no robots.txt, so it names no Sitemap: either", "",
				"write one with: dev seo write <dir> — Google reads a missing file as "+
					"allow-all, but finds no sitemap from it — "+DocRobotsIntro))
		}},
	)
	return func(res tool.Result) (Found, error) {
		found, err := read(res)
		if found.Summary == "" {
			found.Summary = "1 URL against the live rules"
		}
		return found, err
	}
}
