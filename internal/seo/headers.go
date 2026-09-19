// The response headers a page is served with, as a `_headers` file.
//
// Two checkers report these independently — kitsune as `security.*` and scry
// as `security/*` — and until now nothing could fix them: eight findings on a
// real site that a reader could only be told about. They are headers, so the
// file that sets them is the answer, and Cloudflare Pages and Netlify both
// read `_headers` from the site root.
//
// Four of the five are written. The fifth is not, and that is deliberate: a
// Content-Security-Policy that is wrong breaks the page silently, in the
// browser, for everyone, and there is no safe default for a policy whose
// whole job is to describe one site's own sources. The file says so where
// whoever edits it will read it.
package seo

import (
	"strconv"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/seo/checkers"
)

// headers is the set written, each with why it is safe to set blind.
var headers = []struct{ name, value, why string }{
	{"Strict-Transport-Security", "max-age=31536000; includeSubDomains",
		"Search requires HTTPS; this stops the first request being plain http"},
	{"X-Content-Type-Options", "nosniff",
		"a browser guessing a type is how a .txt becomes a script"},
	{"X-Frame-Options", "SAMEORIGIN",
		"someone else framing the page is how a click becomes theirs"},
	{"Referrer-Policy", "strict-origin-when-cross-origin",
		"the path a reader came from is theirs, not the next site's"},
}

func writeHeaders(s Site) (content, covered string, err error) {
	var b strings.Builder
	b.WriteString("# Written by dev seo write. Cloudflare Pages and Netlify read this;\n")
	b.WriteString("# another host wants the same headers said its own way.\n/*\n")
	for _, h := range headers {
		b.WriteString("  " + h.name + ": " + h.value + "\n")
	}
	b.WriteString("\n# Not written, and not an oversight: a Content-Security-Policy\n")
	b.WriteString("# describes one site's own sources, so a default would either\n")
	b.WriteString("# allow everything and mean nothing, or break the page in the\n")
	b.WriteString("# browser where no check here would see it. Write yours:\n")
	b.WriteString("#   Content-Security-Policy: default-src 'self'; ...\n")
	b.WriteString("# " + checkers.DocEssentials + "\n")
	return b.String(), cli.Plural(len(headers), "header") + ", CSP left to you", nil
}

// validateHeaders reads them back. Presence, not policy: whether a value is
// right for a site is that site's business, and whether it is there at all is
// not.
func validateHeaders(name, content, origin string) (found []cli.Finding, covered string) {
	present := 0
	for _, h := range headers {
		if strings.Contains(content, h.name+":") {
			present++
			continue
		}
		found = append(found, cli.Finding{
			Severity: cli.SevWarning,
			ID:       "missing-header-" + strings.ToLower(h.name),
			Message:  "no " + h.name,
			Fix:      h.why + " — set it in " + name + " — " + checkers.DocEssentials,
		})
	}
	if !strings.Contains(content, "/*") {
		found = append(found, cli.Finding{
			Severity: cli.SevError,
			ID:       "headers-no-rule",
			Message:  "no path rule, so none of these headers is applied to anything",
			Fix:      "the file needs a path line such as /* before its headers — " + checkers.DocEssentials,
		})
	}
	return found, cli.Plural(present, "header") + " of " + strconv.Itoa(len(headers))
}
