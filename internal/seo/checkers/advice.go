// What to do about each fault, and where Google explains why.
//
// Every finding this tool produces carries one: an id and a severity say what
// is wrong and not what to do about it, and a report nobody can act on is a
// report nobody reads. Written once here and reached from everywhere —
// whichever checker found it, or the validator that read the file.
package checkers

// Fix is what to do about a fault, and where Google says why.
//
// Keyed by the names scoutly uses because that is where they came from, and
// shared by everything: the other checkers map their own spellings onto
// these to find the advice, and the artifact validators above use it too, so
// what a person is told about a missing canonical is written once whether a
// crawler found it or a file was read from disk. Every
// failure naming its own fix is this repo's rule, and a checker is the place
// it matters most: a person reading "missing-meta-description" already knows
// what is missing and not what Google does with it.
//
// A code with no entry still reports — with the tool's own message and the
// documentation index — because a checker that hides what it cannot explain
// is worse than one that admits the gap.
func Fix(code string) string {
	if fix, ok := fixes[code]; ok {
		return fix
	}
	return "see the checker's own report for this code, and " + DocEssentials
}

const (
	DocEssentials  = "https://developers.google.com/search/docs/essentials"
	DocTitles      = "https://developers.google.com/search/docs/appearance/title-link"
	DocSnippets    = "https://developers.google.com/search/docs/appearance/snippet"
	DocCanonical   = "https://developers.google.com/search/docs/crawling-indexing/consolidate-duplicate-urls"
	DocHeadings    = "https://developers.google.com/search/docs/appearance/structured-data/article"
	DocImages      = "https://developers.google.com/search/docs/appearance/google-images"
	DocRedirects   = "https://developers.google.com/search/docs/crawling-indexing/301-redirects"
	DocCrawling    = "https://developers.google.com/search/docs/crawling-indexing/overview-google-crawlers"
	DocStructured  = "https://developers.google.com/search/docs/appearance/structured-data/intro-structured-data"
	DocRobotsIntro = "https://developers.google.com/search/docs/crawling-indexing/robots/intro"
)

var fixes = map[string]string{
	"missing-title":            "give the page a <title>: it is what Search shows as the result's headline — " + DocTitles,
	"title-too-long":           "shorten the <title>: Search truncates a long one, so the end of it never reaches a reader — " + DocTitles,
	"title-too-short":          "say what the page is in the <title>; one or two words describes nothing to rank — " + DocTitles,
	"duplicate-title":          "give each page its own <title>: two pages with one title compete with each other — " + DocTitles,
	"missing-meta-description": `add <meta name="description" content="..."> — Search writes the snippet under the result from it, and without one it invents one from the page — ` + DocSnippets,
	"missing-h1":               "give the page one <h1> saying what it is about — " + DocHeadings,
	"multiple-h1":              "leave one <h1>: several tell a reader and a crawler different things about what the page is — " + DocHeadings,
	"missing-canonical":        `add <link rel="canonical" href="..."> with the absolute URL this page should be indexed as — ` + DocCanonical,
	"missing-alt":              "give the image an alt: it is what Images indexes and what a screen reader says — " + DocImages,
	"broken-link":              "fix or remove the link: a crawler follows it, finds nothing, and spends the crawl budget doing it — " + DocCrawling,
	"broken-image":             "fix or remove the image: it is a request that costs the page and returns nothing — " + DocImages,
	"redirect":                 "point the link at its final URL: a hop costs a request and dilutes what the link says — " + DocRedirects,
	"blocked-by-robots":        "check robots.txt is meant to block this: Google will not index what it cannot fetch — " + DocRobotsIntro,
	"missing-og-title":         "add og:title so a shared link shows what the page is — " + DocEssentials,
	"missing-og-description":   "add og:description so a shared link says what the page is about — " + DocEssentials,
	"missing-viewport":         `add <meta name="viewport" content="width=device-width, initial-scale=1"> — Search indexes the mobile page, and without it the mobile page is the desktop one — ` + DocEssentials,
	"missing-og-image":         "add og:image so a shared link is not a bare URL — " + DocEssentials,
	"missing-og-url":           "add og:url with the canonical URL, so a shared link credits the page you want indexed and not the one it was copied from — " + DocEssentials,
	"missing-og-type":          "add og:type — website for a page, article for a post — so a preview knows what it is showing — " + DocEssentials,
	// The two the header validator and scry both need a sentence for. Written
	// here rather than beside either, because scry reports them against a
	// deployed URL and the validator reports them against the _headers file,
	// and a reader should be told the same thing whichever found it.
	"csp-unsafe":      "drop 'unsafe-inline' and 'unsafe-eval': a policy that allows inline script or style does not stop the injection the header exists to stop, so it reads as protection and is none — " + DocEssentials,
	"missing-charset": "say the encoding in the type: Content-Type: text/html; charset=utf-8 — a browser left to guess can render the page in the wrong encoding, and what a crawler indexes is what the browser rendered — " + DocEssentials,
	"missing-json-ld": `add a <script type="application/ld+json"> block describing the page — it is what a rich result is built from — ` + DocStructured,
	"thin-content":    "say more on the page: there is not enough here for Search to know what it answers — " + DocEssentials,
}
