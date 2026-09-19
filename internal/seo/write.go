// The write side: the files a site needs for Google to find and describe it,
// and the check that proves each one good.
//
// Every one is written by hand, with no library. That is not laziness: no Go
// package generates robots.txt — every one of them is a parser — and the two
// that generate the rest would each add a dependency to a tool that has two,
// one of them dragging a whole second template engine in to render six meta
// tags. A sitemap is XML and robots.txt is four lines.
//
// A writer declares what it Produces and how to validate it, so `write` and
// `validate` read the same declaration: an artifact cannot pass at write time
// and fail at check time, and a new writer's output cannot go unvalidated.
package seo

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joeblew999/dev/cli"
)

// Writer is one file the site needs. Adding one is an entry here and nothing
// else: write walks this list, and so does validate.
type Writer struct {
	Name     string
	Provides string
	Produces Artifact

	// Fixes are the finding ids this file's existence resolves, by whichever
	// checker reported them. It is what turns a report into a route: a
	// checker says the canonical is missing, and this says which file to
	// write to have one. Matched by prefix, so kitsune's seo.canonical.* and
	// scoutly's missing-canonical both land on the same writer.
	Fixes []string

	// Write returns the file's content and what it put in it, in the words
	// that apply to that file: a sitemap counts URLs, a head fragment names
	// the tags. The report has a column for it either way — a write that says
	// only "0 findings" tells the reader nothing about what it did.
	Write func(w Site) (content, covered string, err error)
}

// Artifact is a file, and the check that says it is good.
type Artifact struct {
	Name string
	// Validate returns what is wrong, and what it looked at — so a report of
	// a clean file still says what was checked rather than leaving a blank.
	Validate func(name, content, origin string) (found []cli.Finding, covered string)
}

// Site is what a writer needs to know: where the site is, and what the page
// says about itself.
type Site struct {
	Origin string // https://example.com
	URL    string // the page the head fragment describes
	Title  string
	Desc   string
	Image  string
	URLs   []string
	Now    time.Time
}

var writers = []Writer{
	{
		Name:     "sitemap",
		Provides: "sitemap.xml — the list of pages Google should fetch",
		Produces: Artifact{Name: "sitemap.xml", Validate: validateSitemap},
		Fixes:    []string{"sitemap-", "seo.sitemap.", "seo/sitemap", "SITEMAP"},
		Write:    writeSitemap,
	},
	{
		Name:     "robots",
		Provides: "robots.txt — what a crawler may fetch, and where the sitemap is",
		Produces: Artifact{Name: "robots.txt", Validate: validateRobots},
		Fixes:    []string{"robots-", "seo.robots_txt.", "seo/robots", "blocked-by-robots"},
		Write:    writeRobots,
	},
	{
		Name:     "headers",
		Provides: "_headers — the response headers two checkers report missing and nothing could fix",
		Produces: Artifact{Name: "_headers", Validate: validateHeaders},
		Fixes: []string{
			"security.", "security/", "missing-header-", "headers-",
			"health/missing-charset",
		},
		Write: writeHeaders,
	},
	{
		Name:     "head",
		Provides: "head.html — title, description, canonical, Open Graph and JSON-LD",
		Produces: Artifact{Name: "head.html", Validate: validateHead},
		// Each checker's own spelling, because none of them is rewritten:
		// scoutly's hyphenated codes, kitsune's dotted ids, scry's slashed
		// ones, seo-audit's SHOUTING ones. A new checker adds its own here.
		Fixes: []string{
			"missing-title", "title-too-", "duplicate-title", "missing-meta-description",
			"missing-canonical", "canonical-", "missing-og-", "missing-json-ld",
			"json-ld-invalid", "thin-content",
			"seo.title.", "seo.description.", "seo.canonical.", "seo.open_graph.",
			"geo.jsonld.", "geo.content_depth.",
			"seo/title-", "seo/missing-canonical", "seo/missing-meta-description",
			"seo/missing-og", "structured-data/",
			"TITLE_", "META_DESCRIPTION", "CANONICAL", "DUPLICATE_TITLE", "VIEWPORT",
		},
		Write: writeHead,
	},
}

// writeSitemap emits a sitemaps.org urlset. The spec is small and the whole
// of it is here: absolute locations, a lastmod in W3C date format, and a
// priority the first URL wins.
func writeSitemap(s Site) (content, covered string, err error) {
	type url struct {
		Loc        string `xml:"loc"`
		LastMod    string `xml:"lastmod,omitempty"`
		ChangeFreq string `xml:"changefreq,omitempty"`
		Priority   string `xml:"priority,omitempty"`
	}
	type urlset struct {
		XMLName xml.Name `xml:"urlset"`
		NS      string   `xml:"xmlns,attr"`
		URLs    []url    `xml:"url"`
	}
	set := urlset{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for i, u := range s.URLs {
		priority := "0.8"
		if i == 0 {
			priority = "1.0"
		}
		set.URLs = append(set.URLs, url{
			Loc:        u,
			LastMod:    s.Now.Format("2006-01-02"),
			ChangeFreq: "weekly",
			Priority:   priority,
		})
	}
	out, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		return "", "", err
	}
	word := "URLs"
	if len(s.URLs) == 1 {
		word = "URL"
	}
	return xml.Header + string(out) + "\n", fmt.Sprintf("%d %s", len(s.URLs), word), nil
}

// writeRobots is allow-all plus the Sitemap directive, which is the only
// line in it Google needs and the one most often missing.
func writeRobots(s Site) (content, covered string, err error) {
	return fmt.Sprintf("User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml\n", s.Origin),
		"allow all, sitemap named", nil
}

// writeHead is the document head a page needs: what Search shows, the
// canonical that stops two URLs competing, and the two structured formats a
// link preview and a rich result are built from.
func writeHead(s Site) (content, covered string, err error) {
	esc := strings.NewReplacer(`&`, "&amp;", `<`, "&lt;", `>`, "&gt;", `"`, "&quot;").Replace
	var b strings.Builder
	fmt.Fprintf(&b, "<title>%s</title>\n", esc(s.Title))
	fmt.Fprintf(&b, "<meta name=\"description\" content=%q>\n", esc(s.Desc))
	fmt.Fprintf(&b, "<link rel=\"canonical\" href=%q>\n", s.URL)
	for _, tag := range [][2]string{
		{"og:type", "website"},
		{"og:title", esc(s.Title)},
		{"og:description", esc(s.Desc)},
		{"og:url", s.URL},
		{"og:image", s.Image},
	} {
		if tag[1] == "" {
			continue
		}
		fmt.Fprintf(&b, "<meta property=%q content=%q>\n", tag[0], tag[1])
	}
	fmt.Fprintf(&b, "<meta name=\"twitter:card\" content=\"summary_large_image\">\n")
	fmt.Fprintf(&b, `<script type="application/ld+json">
{"@context":"https://schema.org","@type":"WebPage","url":%q,"name":%q,"description":%q}
</script>
`, s.URL, s.Title, s.Desc)
	// What a reader wants to know is which of these the page now carries, and
	// og:image is the one that is missing when nobody passed --image.
	tags := "title, description, canonical, Open Graph, JSON-LD"
	if s.Image == "" {
		tags += " (no og:image: pass --image)"
	}
	return b.String(), tags, nil
}

// each walks the writers, honouring --only and --skip, and hands every one
// that runs to do. Write and Validate differed only in what do was: produce
// the content, or read it back. Everything around that — the selection, the
// timing, the step, attributing findings to the writer — was written twice.
func each(rep *cli.Report, pick *picked, do func(Writer) (found []cli.Finding, covered, path string, err error)) int {
	ran := 0
	for _, w := range writers {
		if why := pick.skipped(w.Name); why != "" {
			rep.NotRun(cli.Step{Name: w.Name, Provides: w.Provides}, why)
			continue
		}
		started := time.Now()
		found, covered, path, err := do(w)
		took := time.Since(started)
		step := cli.Step{Name: w.Name, Provides: w.Provides, Covered: covered,
			Took: cli.Took(took), TookMs: took.Milliseconds(), Report: path,
			Findings: len(found)}
		if err != nil {
			rep.NotRun(step, err.Error())
			continue
		}
		ran++
		for _, f := range found {
			f.Tool = w.Name
			rep.Add(f)
		}
		rep.Ran(step)
	}
	return ran
}

// Write writes every artifact into dir and validates what it wrote, with the
// same checks `validate` runs — so nothing can pass here and fail there.
func Write(c cli.Call, dir string, s Site, rep *cli.Report, pick *picked) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	each(rep, pick, func(w Writer) ([]cli.Finding, string, string, error) {
		content, covered, err := w.Write(s)
		if err != nil {
			return nil, "", "", err
		}
		path := filepath.Join(dir, w.Produces.Name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, "", "", err
		}
		if !c.Given("quiet") {
			fmt.Fprintf(c.Stderr, "  wrote %-14s %s\n", w.Produces.Name, w.Provides)
		}
		found, _ := w.Produces.Validate(w.Produces.Name, content, s.Origin)
		return found, covered, path, nil
	})
	return nil
}

// Validate checks artifacts already on disk, with no network at all: the same
// checks Write runs on what it just wrote. Good as a pre-commit hook.
func Validate(dir string, origin string, rep *cli.Report, pick *picked) error {
	ran := each(rep, pick, func(w Writer) ([]cli.Finding, string, string, error) {
		path := filepath.Join(dir, w.Produces.Name)
		data, err := os.ReadFile(path)
		if err != nil {
			rep.Add(cli.Finding{Tool: w.Name, Severity: cli.SevWarning,
				ID: "missing-" + w.Name, Message: w.Produces.Name + " is not in " + dir,
				Fix: "write it with: dev seo write " + dir, FixedBy: w.Name})
			return nil, "", "", fmt.Errorf("%s is not in %s", w.Produces.Name, dir)
		}
		// The origin a strict check compares against is read from the files
		// when nobody named one: guessing a host fails every URL on a
		// spurious host check.
		if origin == "" {
			origin = originFrom(string(data))
		}
		found, covered := w.Produces.Validate(w.Produces.Name, string(data), origin)
		return found, covered, path, nil
	})
	if ran == 0 {
		return fmt.Errorf("%s holds none of the files dev seo writes; make them with: dev seo write %s", dir, dir)
	}
	return nil
}

// originFrom reads a host out of whatever the file holds, so validating a
// directory needs no URL.
func originFrom(content string) string {
	for _, marker := range []string{"<loc>", `href="`, `content="http`} {
		if _, after, ok := strings.Cut(content, marker); ok {
			if before, _, ok := strings.Cut(after, "<"); ok {
				if u := originOf(strings.TrimSpace(before)); u != "" {
					return u
				}
			}
		}
	}
	return ""
}

// originOf is the scheme and host of a URL, which is what every artifact is
// checked against.
func originOf(raw string) string {
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return ""
	}
	host, _, _ := strings.Cut(rest, "/")
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

// fixedBy is the writer whose output resolves a finding, or "" when none
// does. A broken link is content and no file fixes it; a missing canonical is
// a tag this command writes.
func fixedBy(id string) string {
	for _, w := range writers {
		for _, prefix := range w.Fixes {
			if strings.HasPrefix(id, prefix) {
				return w.Name
			}
		}
	}
	return ""
}
