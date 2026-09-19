// ldlint validates JSON-LD against the schema.org vocabulary itself: not
// whether a block is present, which every other checker here can tell you,
// but whether the properties in it are valid on the type they are attached
// to. That is the check this package could not write — schema.org publishes
// no JSON Schema, so validating it means carrying the vocabulary, which is
// what ldlint does and what made importing a validator not worth it.
//
// Text output, not JSON: it prints a count line and an ERROR line per
// problem, and exits 1 when it found any.
package checkers

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
	Read:     fromLdlint(),
}

// The prose it prints: a count line, and an ERROR line per problem.
func fromLdlint() func(tool.Result) (Found, error) {
	return Text(
		Rule{When: Has("ERROR"), Then: func(line string, f *Found) {
			f.Issues = append(f.Issues, Issue("ldlint", "json-ld-invalid", cli.SevError,
				strings.TrimSpace(strings.TrimPrefix(line, "ERROR")), "",
				"use a property schema.org defines on that type, or change the type — "+
					"a rich result is built from this and Google ignores what it cannot read — "+DocStructured))
		}},
		// No JSON-LD at all. The head writer produces one, so this is a gap
		// with a route out rather than an error in what is there.
		Rule{When: Has("0 schemas"), Then: func(line string, f *Found) {
			f.Summary = "no structured data to check"
			f.Issues = append(f.Issues, Issue("ldlint", "missing-json-ld", cli.SevWarning,
				"no JSON-LD on the page", "", Fix("missing-json-ld")))
		}},
		Rule{When: Has("schemas"), Then: func(line string, f *Found) { f.Summary = line }},
	)
}
