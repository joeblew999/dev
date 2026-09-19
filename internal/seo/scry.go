// scry is the broad one: 90-odd checks over seo, security, performance,
// accessibility, images and structured data, on every page it crawls.
//
// Two things the seocheck catalogue reports about it are not true of the
// version pinned here, and both were found by reading what it actually
// printed rather than its README:
//
//   - It is said to emit no rule ids, every issue carrying `rule_id: null`.
//     There is no rule_id field at all; the id is `check_name`, it is
//     populated on every issue, and it reads `category/name` —
//     `seo/missing-meta-description`. That is a usable id for a CI rule.
//   - It is said to need `--output-file` for JSON. `-o json` writes to stdout,
//     which is what this uses; its own logs go to stderr where they belong.
package seo

import (
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
)

var scry = Checker{
	Name:     "scry",
	Pin:      `"go:github.com/meysam81/scry" = "latest"`,
	Provides: "90-odd checks a page at a time, including TLS expiry and the security headers",
	Cost:     "~1s",
	Args: func(a Ask) []string {
		return []string{"check", a.URL, "-o", "json"}
	},
	Read: readScry,
}

type scryReport struct {
	Pages  []struct{} `json:"pages"`
	Issues []struct {
		Check    string `json:"check_name"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
		URL      string `json:"url"`
	} `json:"issues"`
}

func readScry(res tool.Result) (Found, error) {
	report, err := res.JSON[scryReport]("scry's report")
	if err != nil {
		return Found{}, err
	}
	found := Found{Pages: len(report.Pages)}
	for _, i := range report.Issues {
		// Its category is the first half of the check name, and the same two
		// that kitsune's are filtered on are dropped here: accessibility is a
		// different subject, and this verb is about what Google asks of a page.
		category, _, _ := strings.Cut(i.Check, "/")
		if i.Severity == cli.SevInfo || category == "accessibility" {
			continue
		}
		found.Issues = append(found.Issues, cli.Finding{
			Tool:     "scry",
			ID:       i.Check,
			Severity: i.Severity,
			Message:  i.Message,
			Where:    i.URL,
			Fix:      scryFix(i.Check),
		})
	}
	return found, nil
}

// scryFix maps its names onto the advice this package already writes, and
// falls back to naming the check when there is none. scry says what is wrong
// and not what to do, which is the gap this fills.
func scryFix(check string) string {
	_, name, _ := strings.Cut(check, "/")
	if fix := scoutlyFixes[name]; fix != "" {
		return fix
	}
	switch name {
	case "cert-expiring-soon", "cert-expired":
		return "renew the certificate: Search requires HTTPS, and a browser will refuse the page before Google ranks it — " + docEssentials
	case "missing-hsts", "missing-csp", "missing-x-content-type-options", "missing-referrer-policy":
		return "add the header at the edge or in the server config — " + docEssentials
	}
	return "see scry's report for " + check + ", and " + docEssentials
}
