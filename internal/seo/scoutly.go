// scoutly is the crawl and on-page tool: it fetches the pages, reads what
// Google reads — title, meta description, H1, canonical, Open Graph, images —
// and reports one issue per thing it found. This file is everything that
// knows scoutly exists: its flags, the JSON it prints, and what a person
// should do about each code it can emit. A second tool is a second file like
// this one, and nothing else changes.
package seo

import (
	"fmt"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
)

// scoutlyReport is the subset of scoutly's JSON this reads. A subset on
// purpose: a field nobody uses is a field that breaks the build when the tool
// renames it, and encoding/json ignores what is not declared.
type scoutlyReport struct {
	URL     string `json:"url"`
	Summary struct {
		Pages int `json:"pages"`
		Links struct {
			Total      int `json:"total"`
			Checked    int `json:"checked"`
			Broken     int `json:"broken"`
			Redirected int `json:"redirected"`
		} `json:"links"`
		Issues struct {
			Total   int `json:"total"`
			Error   int `json:"error"`
			Warning int `json:"warning"`
			Info    int `json:"info"`
		} `json:"issues"`
	} `json:"summary"`
	Issues []scoutlyIssue `json:"issues"`
}

type scoutlyIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Target   struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"target"`
}

// scoutly is the crawl and on-page checker: it reads what Google reads on
// each page and reports one issue per thing it found.
var scoutly = Checker{
	Name:     "scoutly",
	Pin:      `"go:github.com/nelsonlaidev/scoutly/cmd/scoutly" = "v0.5.0"`,
	Provides: "what Google reads on each page: title, meta description, H1, canonical, Open Graph, images",
	Cost:     "~1s for a few pages",
	Args: func(a Ask) []string {
		args := []string{a.URL, "--format", "json"}
		if a.Pages > 0 {
			args = append(args, "--max-pages", fmt.Sprint(a.Pages))
		}
		return args
	},
	Read: readScoutly,
}

func readScoutly(res tool.Result) (Found, error) {
	report, err := res.JSON[scoutlyReport]("scoutly's report")
	if err != nil {
		return Found{}, err
	}
	return Found{
		Pages:  report.Summary.Pages,
		Links:  report.Summary.Links.Checked,
		Broken: report.Summary.Links.Broken,
		Issues: cli.Map(report.Issues, func(i scoutlyIssue) cli.Finding {
			return cli.Finding{
				Tool:     "scoutly",
				ID:       i.Code,
				Severity: i.Severity,
				Message:  i.Message,
				Where:    i.Target.URL,
				Fix:      scoutlyFix(i.Code),
			}
		}),
	}, nil
}

// scoutlyFix is what to do about a code, and where Google says why. Named
// for where the codes came from, but shared: seo-audit and ldlint map their
// own names onto these, so advice written once is reached from all of them. Every
// failure naming its own fix is this repo's rule, and a checker is the place
// it matters most: a person reading "missing-meta-description" already knows
// what is missing and not what Google does with it.
//
// A code with no entry still reports — with the tool's own message and the
// documentation index — because a checker that hides what it cannot explain
// is worse than one that admits the gap.
func scoutlyFix(code string) string {
	if fix, ok := scoutlyFixes[code]; ok {
		return fix
	}
	return "see the checker's own report for this code, and " + docEssentials
}

const (
	docEssentials  = "https://developers.google.com/search/docs/essentials"
	docTitles      = "https://developers.google.com/search/docs/appearance/title-link"
	docSnippets    = "https://developers.google.com/search/docs/appearance/snippet"
	docCanonical   = "https://developers.google.com/search/docs/crawling-indexing/consolidate-duplicate-urls"
	docHeadings    = "https://developers.google.com/search/docs/appearance/structured-data/article"
	docImages      = "https://developers.google.com/search/docs/appearance/google-images"
	docRedirects   = "https://developers.google.com/search/docs/crawling-indexing/301-redirects"
	docCrawling    = "https://developers.google.com/search/docs/crawling-indexing/overview-google-crawlers"
	docRobotsIntro = "https://developers.google.com/search/docs/crawling-indexing/robots/intro"
)

var scoutlyFixes = map[string]string{
	"missing-title":            "give the page a <title>: it is what Search shows as the result's headline — " + docTitles,
	"title-too-long":           "shorten the <title>: Search truncates a long one, so the end of it never reaches a reader — " + docTitles,
	"title-too-short":          "say what the page is in the <title>; one or two words describes nothing to rank — " + docTitles,
	"duplicate-title":          "give each page its own <title>: two pages with one title compete with each other — " + docTitles,
	"missing-meta-description": `add <meta name="description" content="..."> — Search writes the snippet under the result from it, and without one it invents one from the page — ` + docSnippets,
	"missing-h1":               "give the page one <h1> saying what it is about — " + docHeadings,
	"multiple-h1":              "leave one <h1>: several tell a reader and a crawler different things about what the page is — " + docHeadings,
	"missing-canonical":        `add <link rel="canonical" href="..."> with the absolute URL this page should be indexed as — ` + docCanonical,
	"missing-alt":              "give the image an alt: it is what Images indexes and what a screen reader says — " + docImages,
	"broken-link":              "fix or remove the link: a crawler follows it, finds nothing, and spends the crawl budget doing it — " + docCrawling,
	"broken-image":             "fix or remove the image: it is a request that costs the page and returns nothing — " + docImages,
	"redirect":                 "point the link at its final URL: a hop costs a request and dilutes what the link says — " + docRedirects,
	"blocked-by-robots":        "check robots.txt is meant to block this: Google will not index what it cannot fetch — " + docRobotsIntro,
	"missing-og-title":         "add og:title so a shared link shows what the page is — " + docEssentials,
	"missing-og-description":   "add og:description so a shared link says what the page is about — " + docEssentials,
	"missing-viewport":         `add <meta name="viewport" content="width=device-width, initial-scale=1"> — Search indexes the mobile page, and without it the mobile page is the desktop one — ` + docEssentials,
	"missing-og-image":         "add og:image so a shared link is not a bare URL — " + docEssentials,
	"missing-json-ld":          `add a <script type="application/ld+json"> block describing the page — it is what a rich result is built from — ` + docStructured,
	"thin-content":             "say more on the page: there is not enough here for Search to know what it answers — " + docEssentials,
}
