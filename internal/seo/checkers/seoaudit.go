// seo-audit is the scored checker: it runs a fixed list of checks over each
// page and gives the site a number out of a hundred. Its shape is a third
// one again — a check carries how many pages failed it, rather than a page
// carrying what it failed — and the score it produces is the kind of thing a
// merged report cannot model, so it stays in the sub-report and the link to
// it is how a reader gets there.
package checkers

import (
	"fmt"

	"github.com/joeblew999/dev/cli"
)

var seoAudit = Checker{
	Name:     "seo-audit",
	Spec:     "go:github.com/Erose112/seo-audit@latest",
	Provides: "a fixed list of checks scored across the site, and duplicate titles between pages",
	Cost:     "~600ms",
	Args: func(a Ask) []string {
		return Paged([]string{"crawl", "--url", a.URL, "--output", FormatJSON}, a.Pages)
	},
	Read: JSON("seo-audit's report", fromSEOAudit),
}

type seoAuditReport struct {
	Score        int `json:"score"`
	PagesCrawled int `json:"pages_crawled"`
	Checks       []struct {
		ID          string `json:"check_id"`
		Name        string `json:"name"`
		Severity    string `json:"severity"`
		PagesFailed int    `json:"pages_failed"`
		PagesTotal  int    `json:"pages_total"`
	} `json:"checks"`
	Summary struct {
		BrokenLinks int `json:"broken_links"`
	} `json:"summary"`
}

func fromSEOAudit(report seoAuditReport) Found {
	// Only the checks that failed something are issues; a check every page
	// passed is not news.
	failed := cli.Filter(report.Checks, func(c struct {
		ID          string `json:"check_id"`
		Name        string `json:"name"`
		Severity    string `json:"severity"`
		PagesFailed int    `json:"pages_failed"`
		PagesTotal  int    `json:"pages_total"`
	}) bool {
		return c.PagesFailed > 0
	})
	found := Found{
		Pages:  report.PagesCrawled,
		Broken: report.Summary.BrokenLinks,
	}
	for _, c := range failed {
		found.Issues = append(found.Issues, cli.Finding{
			Tool: "seo-audit",
			// Its own id, not a translation of it: a report says what the
			// checker said, and a CI rule matches on what the checker emits.
			// The mapping below is only how the advice is found.
			ID:       c.ID,
			Severity: c.Severity,
			Message:  fmt.Sprintf("%s: %d of %d page(s)", c.Name, c.PagesFailed, c.PagesTotal),
			Fix:      Fix(seoAuditCode(c.ID)),
		})
	}
	return found
}

// seoAuditCode finds the advice this package already wrote for a fault this
// checker names differently. It is not used to rename the finding: the id in
// the report is the one seo-audit emitted, because a report that rewrites
// what a tool said is a report you cannot check against the tool.
func seoAuditCode(id string) string {
	if code, ok := seoAuditCodes[id]; ok {
		return code
	}
	return id
}

var seoAuditCodes = map[string]string{
	"TITLE_EXISTS":     "missing-title",
	"TITLE_LENGTH":     "title-too-long",
	"META_DESCRIPTION": "missing-meta-description",
	"SINGLE_H1":        "multiple-h1",
	"IMAGE_ALT":        "missing-alt",
	"CANONICAL":        "missing-canonical",
	"VIEWPORT":         "missing-viewport",
	"DUPLICATE_TITLE":  "duplicate-title",
	"BROKEN_LINK":      BrokenLink,
}
