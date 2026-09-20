// muffet is the link layer: it follows every link on every page and says
// which ones do not answer. Its JSON looks nothing like the other checkers' —
// a page carries a list of links, and a link carries the error it produced —
// which is the point: each checker keeps its own shape, and only what they
// all have in common reaches the merged report.
package checkers

import (
	"fmt"
	"strings"

	"github.com/joeblew999/dev/cli"
)

var muffet = Checker{
	Name:     "muffet",
	Spec:     "go:github.com/raviqqe/muffet/v2@v2.11.5",
	Provides: "every link on every page followed, and which ones do not answer",
	Cost:     "~3s, the slowest of the three",
	Args: func(a Ask) []string {
		return []string{
			a.URL,
			"--format", FormatJSON,
			// Not --follow-robots-txt: muffet treats a 404 robots.txt as a
			// failure and stops, while Google treats it as allow-all. A site
			// without one is normal, not a reason to learn nothing.
			// muffet's buffer is smaller than some hosts' response headers,
			// and the failure reads as a broken link when the page is fine.
			"--buffer-size", "16384",
			"--max-connections", "16",
			// A fragment is client-side state, not another page: without this
			// one playground link with sixty saved states reads as sixty
			// broken links, and the report is noise.
			"--ignore-fragments",
		}
	},
	Read: JSON("muffet's report", fromMuffet),
}

// muffetPage is one crawled page and what its links did. Only the fields this
// report shares are declared; the rest stays in the sub-report.
type muffetPage struct {
	URL   string `json:"url"`
	Links []struct {
		URL   string `json:"url"`
		Error string `json:"error"`
	} `json:"links"`
}

func fromMuffet(pages []muffetPage) Found {
	// muffet lists only the pages that have something wrong, so counting them
	// as pages crawled would report a clean site as an empty one. What it
	// knows is what is broken; the crawl counts belong to the checkers that
	// report every page.
	var found Found
	for _, page := range pages {
		for _, link := range page.Links {
			if link.Error == "" {
				continue
			}
			found.Broken++
			found.Issues = append(found.Issues, cli.Finding{
				Tool:     "muffet",
				ID:       muffetCode(link.Error),
				Severity: "error",
				Message:  fmt.Sprintf("%s → %s", link.URL, firstLine(link.Error)),
				Where:    page.URL,
				Fix:      "fix or remove the link: a crawler follows it, finds nothing, and spends the crawl budget doing it — " + DocCrawling,
			})
		}
	}
	// Said in muffet's own terms: it saw every link on every page, and what it
	// reports is only what failed. No counts of its own, so no counts here.
	found.Summary = "no broken links"
	if found.Broken > 0 {
		found.Summary = fmt.Sprintf("%d broken links", found.Broken)
		if found.Broken == 1 {
			found.Summary = "1 broken link"
		}
	}
	return found
}

// muffetCode names the kind of failure, since muffet reports a sentence and a
// report is easier to act on when the same failure has the same name.
func muffetCode(err string) string {
	switch {
	case strings.Contains(err, "404"):
		return BrokenLink
	case strings.Contains(err, "timeout") || strings.Contains(err, "deadline"):
		return "slow-link"
	case strings.Contains(err, "certificate") || strings.Contains(err, "tls"):
		return "insecure-link"
	default:
		return "unreachable-link"
	}
}

// firstLine keeps a message to one line: muffet quotes whole response headers
// into an error, and a report is read at a glance.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	if len(line) > 120 {
		line = line[:117] + "..."
	}
	return line
}
