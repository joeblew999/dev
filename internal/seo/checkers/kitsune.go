// kitsune is the per-page auditor with the precise names. Where the other
// checkers report a sentence or a SHOUTING id, kitsune reports dotted ids —
// seo.title.short, seo.canonical.missing — which is what a CI rule should
// match on, because they do not change when the wording does.
//
// It fetches /robots.txt and /sitemap.xml itself, so it sees a missing
// sitemap that a page-only checker cannot.
package checkers

import (
	"strings"

	"github.com/joeblew999/dev/cli"
)

var kitsune = Checker{
	Name:     "kitsune",
	Pin:      `"go:github.com/berkaycubuk/kitsune/cmd/kitsune" = "latest"`,
	Provides: "per-page findings with stable dotted ids, and Google's own guideline link for each",
	Cost:     "~300ms, the fastest of them",
	Args: func(a Ask) []string {
		// One page, whatever --max-pages says: kitsune audits the URL given
		// and does not crawl. Saying so here is better than a flag that
		// quietly does nothing.
		return []string{"--json", a.URL}
	},
	Read: JSON("kitsune's report", fromKitsune),
}

type kitsuneReport struct {
	URL     string `json:"url"`
	Status  int    `json:"status_code"`
	Results []struct {
		ID       string `json:"id"`
		Category string `json:"category"`
		Severity string `json:"severity"`
		Title    string `json:"title"`
		Detail   string `json:"detail"`
		Fix      string `json:"recommendation"`
		Doc      string `json:"guideline_url"`
	} `json:"results"`
}

// inRemit is which of kitsune's categories this verb is about. It reports
// five, and two of them are not Google Search conformance: geo is llms.txt
// and the AI crawlers, which this verb parks as an explicit non-goal, and
// a11y is a different subject with its own tools.
//
// Nothing is lost by leaving them out — kitsune's own report is written
// beside this one in full, so anyone who wants them has them. That is what
// keeping each checker's output is for.
var inRemit = map[string]bool{
	"seo": true,
	// Page experience is part of what Google ranks on, so these two stay.
	"perf":     true,
	"security": true,
}

func fromKitsune(report kitsuneReport) Found {
	found := Found{Pages: 1, Summary: "1 page, every check Google names"}
	for _, r := range report.Results {
		// It reports what passed as well as what failed, and the passes are
		// most of them — 39 of 55 on a small site. A report that lists them
		// buries the four things to fix.
		if r.Severity == cli.SevInfo || !inRemit[r.Category] {
			continue
		}
		found.Issues = append(found.Issues, cli.Finding{
			Tool:     "kitsune",
			ID:       r.ID,
			Severity: r.Severity,
			Message:  orElse(r.Detail, r.Title),
			Where:    report.URL,
			Fix:      kitsuneFix(r.Fix, r.Doc),
		})
	}
	return found
}

// kitsuneFix is what kitsune says to do, and where it says Google explains
// it. It is the only checker here that carries both, so its own words are
// used rather than this package's table.
func kitsuneFix(recommendation, doc string) string {
	fix := strings.TrimSpace(recommendation)
	if fix == "" {
		fix = "see kitsune's report for this id"
	}
	if doc != "" {
		fix += " — " + doc
	}
	return fix
}
