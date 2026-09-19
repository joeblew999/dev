// ldlint validates JSON-LD against the schema.org vocabulary itself: not
// whether a block is present, which every other checker here can tell you,
// but whether the properties in it are valid on the type they are attached
// to. That is the check this package could not write — schema.org publishes
// no JSON Schema, so validating it means carrying the vocabulary, which is
// what ldlint does and what made importing a validator not worth it.
//
// Text output, not JSON: it prints a count line and an ERROR line per
// problem, and exits 1 when it found any.
package seo

import (
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
)

var ldlint = Checker{
	Name:     "ldlint",
	Pin:      `"go:github.com/kevhq/ldlint/cmd/ldlint" = "latest"`,
	Provides: "schema.org vocabulary validation: a property that is not valid on the type it is on",
	Cost:     "~700ms",
	Args:     func(a Ask) []string { return []string{a.URL} },
	Read:     readLdlint,
}

func readLdlint(res tool.Result) (Found, error) {
	var found Found
	for _, line := range cli.Lines(res.Out) {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "ERROR"):
			found.Issues = append(found.Issues, cli.Finding{
				Tool:     "ldlint",
				ID:       "json-ld-invalid",
				Severity: cli.SevError,
				Message:  strings.TrimSpace(strings.TrimPrefix(line, "ERROR")),
				Fix: "use a property schema.org defines on that type, or change the " +
					"type — a rich result is built from this and Google ignores what it " +
					"cannot read — " + docStructured,
			})
		case strings.Contains(line, "0 schemas"):
			// No JSON-LD at all. The head writer produces one, so this is a
			// gap with a route out rather than an error in what is there.
			found.Issues = append(found.Issues, cli.Finding{
				Tool:     "ldlint",
				ID:       "missing-json-ld",
				Severity: cli.SevWarning,
				Message:  "no JSON-LD on the page",
				Fix:      scoutlyFix("missing-json-ld"),
			})
			found.Summary = "no structured data to check"
		case strings.Contains(line, "schemas"):
			found.Summary = line
		}
	}
	if found.Summary == "" {
		found.Summary = "checked the page's structured data"
	}
	return found, nil
}

const docStructured = "https://developers.google.com/search/docs/appearance/structured-data/intro-structured-data"
