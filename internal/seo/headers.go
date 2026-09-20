// The response headers a page is served with, as a `_headers` file.
//
// Three checkers report these independently — kitsune as `security.*` and
// `perf.*`, scry as `security/*` and `health/*` — and until this existed
// nothing could fix them: findings on a real site that a reader could only be
// told about. They are headers, so the file that sets them is the answer, and
// Cloudflare Pages, Cloudflare Workers static assets and Netlify all read
// `_headers` from the site root.
//
// All but one of them are written. The Content-Security-Policy is not, and
// that is deliberate: a policy that is wrong breaks the page silently, in the
// browser, for everyone, and there is no safe default for a policy whose
// whole job is to describe one site's own sources. The file says so where
// whoever edits it will read it.
package seo

import (
	"slices"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/seo/checkers"
)

const (
	// everywhere is the path rule the headers that are true of every response
	// are set on, whatever that response is.
	everywhere = "/*"
	// htmlUTF8 is what an HTML response should say it is. A browser reads the
	// <meta charset> in the document and is fine either way; a crawler, a
	// feed reader and anything else that decodes bytes before parsing them
	// only has the header, and the header saying nothing means each of them
	// guesses.
	htmlUTF8 = "text/html; charset=utf-8"
)

// htmlPaths are the URLs a static site serves HTML at, which is the set that
// wants a charset and nothing else does.
//
// A host works the type out from the file's extension, and the two shapes a
// static site serves pages at have no extension to work from: the root, and a
// directory-style URL like /cli/ that resolves to its index.html. Those are
// the two the edge serves as bare `text/html`. The third is the explicit one,
// for a site that does not use directory URLs.
//
// Deliberately not `/*`: a charset on sitemap.xml or on the icon would be a
// header claiming a file is something it is not, which is the exact fault
// X-Content-Type-Options exists to stop mattering.
var htmlPaths = []string{"/", "/*/", "/*.html"}

// headers is the set written: which paths each is set on, and why it is safe
// to set blind.
//
// The path is part of the declaration rather than implied, because not every
// header belongs everywhere. One table either way, so the writer emits it and
// the validator reads it back without either one keeping a second list — the
// thing that made the whole registry worth having.
var headers = []struct {
	paths            []string
	name, value, why string
}{
	{[]string{everywhere}, "Strict-Transport-Security", "max-age=31536000; includeSubDomains",
		"Search requires HTTPS; this stops the first request being plain http"},
	{[]string{everywhere}, "X-Content-Type-Options", "nosniff",
		"a browser guessing a type is how a .txt becomes a script"},
	{[]string{everywhere}, "X-Frame-Options", "SAMEORIGIN",
		"someone else framing the page is how a click becomes theirs"},
	{[]string{everywhere}, "Referrer-Policy", "strict-origin-when-cross-origin",
		"the path a reader came from is theirs, not the next site's"},
	// One value, on one path, and the number is the argument.
	//
	// A host's default for a static asset is `max-age=0, must-revalidate`:
	// never stale, and a conditional request before every single file on
	// every single page view. Sixty seconds is the smallest number that buys
	// the obvious thing back — a reader clicking between two pages of a
	// manual re-uses what they already have — and it is short enough that a
	// deploy is visible almost at once, which matters when the loop is
	// check, fix, deploy, check.
	//
	// It is not longer, and not different per path, because a longer TTL is
	// only safe on a name that changes when its content does. A site that
	// fingerprints its assets can say so itself; this cannot know that it
	// does, and a year on an un-fingerprinted stylesheet is a reader stuck
	// with last month's page and no way to ask for a new one. must-revalidate
	// is the other half: once the minute is up the browser asks, and the
	// ETag makes the answer a 304 with no body in it.
	{[]string{everywhere}, "Cache-Control", "public, max-age=60, must-revalidate",
		"a host's default asks again for every file on every view, and never caching is not free"},
	{htmlPaths, "Content-Type", htmlUTF8,
		"an extensionless URL is served as bare text/html, and a charset nobody states is a charset guessed"},
}

// blocks are the paths this file is made of, in the order the table first
// mentions each one. Derived rather than listed, so adding a header on a new
// path is one line in one table and not two lines in two.
func blocks() []string {
	var out []string
	for _, h := range headers {
		for _, p := range h.paths {
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

func writeHeaders(s Site) (content, covered string, err error) {
	var b strings.Builder
	b.WriteString("# Written by dev seo write. Cloudflare Pages, Cloudflare Workers\n")
	b.WriteString("# static assets and Netlify all read this; another host wants the\n")
	b.WriteString("# same headers said its own way.\n")
	for i, path := range blocks() {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(path + "\n")
		for _, h := range headers {
			if slices.Contains(h.paths, path) {
				b.WriteString("  " + h.name + ": " + h.value + "\n")
			}
		}
		if path == everywhere {
			b.WriteString(csp(s))
		}
	}
	covered = cli.Plural(len(headers), "header") + " over " + cli.Plural(len(blocks()), "path")
	if s.CSP != "" {
		return b.String(), covered + ", CSP as given", nil
	}
	return b.String(), covered + ", CSP left to you", nil
}

// csp is the one header with no safe default and exactly one right answer per
// site, so it is written when it is given and explained when it is not. Two
// checkers report it missing, and until --csp existed the only thing a reader
// could do with that finding was edit the file this verb had just written —
// which the next write would overwrite.
func csp(s Site) string {
	if s.CSP != "" {
		return "  Content-Security-Policy: " + s.CSP + "\n"
	}
	return "\n# Not written, because none was given: a Content-Security-Policy\n" +
		"# describes one site's own sources, so a default would either\n" +
		"# allow everything and mean nothing, or break the page in the\n" +
		"# browser where no check here would see it. Pass yours:\n" +
		"#   dev seo write . --csp \"default-src 'self'\"\n" +
		"#\n" +
		"# And write it so it does not need 'unsafe-inline': a policy that\n" +
		"# allows every inline style on the page is a policy that stops the\n" +
		"# injected one it was written for. Either link the stylesheet, or\n" +
		"# name the inline block's sha256 in the policy.\n" +
		"# " + checkers.DocEssentials + "\n"
}

// The check that reads this file back is validateHeaders, in validate.go with
// the other four: this file is the write side, and a validator belongs beside
// the validators it shares its helpers with. It walks this same table, so a
// header added here is checked here from the moment it exists.
