// scoutly is the crawl and on-page tool: it fetches the pages, reads what
// Google reads — title, meta description, H1, canonical, Open Graph, images —
// and reports one issue per thing it found. This file is everything that
// knows scoutly exists: its flags, the JSON it prints, and what a person
// should do about each code it can emit. A second tool is a second file like
// this one, and nothing else changes.
package checkers

import (
	"github.com/joeblew999/dev/cli"
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
	Spec:     "go:github.com/nelsonlaidev/scoutly/cmd/scoutly@v0.5.0",
	Provides: "what Google reads on each page: title, meta description, H1, canonical, Open Graph, images",
	Cost:     "~1s for a few pages",
	Args: func(a Ask) []string {
		return Paged([]string{a.URL, "--format", FormatJSON}, a.Pages)
	},
	Read: JSON("scoutly's report", fromScoutly),
}

func fromScoutly(report scoutlyReport) Found {
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
				Fix:      Fix(i.Code),
			}
		}),
	}
}
