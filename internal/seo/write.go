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
	"net/url"
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

	// Needs are the artifacts this writer's own output asserts exist, by the
	// file name that produces them.
	//
	// robots.txt ends with "Sitemap: <origin>/sitemap.xml" and llms.txt points
	// at the same file — both are claims about a file another writer makes.
	// Until this existed the claim was true only because the registry happened
	// to list sitemap first, and `--only robots` wrote a robots.txt naming a
	// sitemap that was never written, and reported pass. A chain held by the
	// order of a slice literal is a chain nobody is holding.
	//
	// One declaration, three things read from it: the order writers run in,
	// what a selection has to pull in with them, and a test that the graph is
	// real and acyclic.
	Needs []string

	// Pages says this writer's subject is which pages exist, so it has
	// something to answer for when nobody gave it a page list. Needs is about
	// the artifacts a writer depends on; this is about the input it depends
	// on, and they fail differently — a missing artifact is a broken chain, a
	// missing page list is a file that is valid and wrong.
	Pages bool

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
	// CSP is the Content-Security-Policy this site wants, when it has one.
	// The only header with no safe default and exactly one right answer per
	// site, so it is the one thing _headers takes rather than decides.
	CSP string
}

var writers = []Writer{
	{
		Name:     "sitemap",
		Provides: "sitemap.xml — the list of pages Google should fetch",
		Produces: Artifact{Name: SitemapFile, Validate: validateSitemap},
		Pages:    true,
		Fixes:    []string{"sitemap-", "seo.sitemap.", "seo/sitemap", "SITEMAP"},
		Write:    writeSitemap,
	},
	{
		Name:     "llms",
		Provides: "llms.txt — the site in the shape a language model reads it",
		Produces: Artifact{Name: "llms.txt", Validate: validateLlms},
		Needs:    []string{SitemapFile},
		Pages:    true,
		Fixes:    []string{"llms-", "seo.llms."},
		Write:    writeLlms,
	},
	{
		Name:     "robots",
		Provides: "robots.txt — what a crawler may fetch, and where the sitemap is",
		Produces: Artifact{Name: "robots.txt", Validate: validateRobots},
		Needs:    []string{SitemapFile},
		Fixes:    []string{"robots-", "seo.robots_txt.", "seo/robots", "blocked-by-robots"},
		Write:    writeRobots,
	},
	{
		Name:     "headers",
		Provides: "_headers — the response headers three checkers report missing and nothing could fix",
		Produces: Artifact{Name: "_headers", Validate: validateHeaders},
		// Caching and the charset are here for the same reason the security
		// four are: they are decided by a response header and by nothing in
		// the page, so this is the only file that can answer them.
		Fixes: []string{
			"security.", "security/", "missing-header-", "headers-",
			"health/missing-charset", "perf.cache_control.",
		},
		Write: writeHeaders,
	},
	{
		Name:     "icon",
		Provides: iconFile + " — the mark a tab, a home screen and a link preview all ask for",
		Produces: Artifact{Name: iconFile, Validate: validateIcon},
		// No Fixes of its own, and that is deliberate rather than forgotten.
		// Every checker reports the missing <link rel="icon"> and the missing
		// og:image, which are tags in the document head — so the finding
		// routes to the writer that emits the tags, and this one is reached
		// through that writer's Needs. The chain already knows how to walk
		// that, and it is the only route that cannot write a tag pointing at
		// a file nobody wrote.
		Write: writeIcon,
	},
	{
		Name:     "head",
		Provides: "head.html — title, description, canonical, icons, Open Graph and JSON-LD",
		Produces: Artifact{Name: "head.html", Validate: validateHead},
		// The two icon tags and og:image all point at the icon writer's file,
		// so writing this without it produces a head that promises a mark the
		// site does not serve.
		Needs: []string{iconFile},
		// Each checker's own spelling, because none of them is rewritten:
		// scoutly's hyphenated codes, kitsune's dotted ids, scry's slashed
		// ones, seo-audit's SHOUTING ones. A new checker adds its own here.
		Fixes: []string{
			"missing-title", "title-too-", "duplicate-title", "missing-meta-description",
			"missing-canonical", "canonical-", "missing-og-", "missing-json-ld",
			"json-ld-invalid", "thin-content",
			"seo.title.", "seo.description.", "seo.canonical.", "seo.open_graph.",
			"seo.icons.", "geo.jsonld.", "geo.content_depth.",
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
// onePage is the warning every writer built from the page list owes the
// reader when there is no page list.
//
// sitemapURLs falls back to the single --url when --urls names no file, so a
// sitemap and an llms.txt could be written for a whole site and list one
// page, and the report said pass — the file was valid, and wrong. A writer
// whose entire subject is which pages exist cannot be silent about having
// been given none.
func onePage(w Writer, s Site) []cli.Finding {
	if !w.Pages || len(s.URLs) > 1 {
		return nil
	}
	return []cli.Finding{{
		Severity: cli.SevWarning, ID: "one-page", Where: w.Produces.Name,
		Message: w.Produces.Name + " lists one page, because --url is all it was given",
		Fix:     "pass --urls FILE, one page per line, when the site has more than one",
	}}
}

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
	return xml.Header + string(out) + "\n", cli.Plural(len(s.URLs), "URL"), nil
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
	// Both rels point at the one file, because it is the one mark: a tab
	// wants 16px of it and an iOS home screen wants 180, and a 512 square
	// scales to either. Two files would be two things to keep the same.
	fmt.Fprintf(&b, "<link rel=\"icon\" href=\"/%s\">\n", iconFile)
	fmt.Fprintf(&b, "<link rel=\"apple-touch-icon\" href=\"/%s\">\n", iconFile)
	for _, tag := range [][2]string{
		{"og:type", "website"},
		{"og:title", esc(s.Title)},
		{"og:description", esc(s.Desc)},
		{"og:url", s.URL},
		{"og:image", socialImage(s)},
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
	// og:image is the one whose answer depends on what was given.
	tags := "title, description, canonical, icon, Open Graph, JSON-LD"
	if s.Image == "" {
		tags += " (og:image is the icon)"
	}
	return b.String(), tags, nil
}

// socialImage is the picture a shared link previews.
//
// --image is a real card: 1200×630 with the site's own words on it, and
// nothing here can invent one. But the icon writer guarantees a square mark
// on every site, and a preview carrying the site's mark beats the grey
// rectangle a missing og:image gets — so the flag is the better answer and
// this is the one that is always true.
//
// Absolute, because a scraper does not resolve a relative og:image. With no
// origin there is no absolute URL to build, and an og:image that cannot be
// fetched is worse than none: it is a broken image where a preview would be.
func socialImage(s Site) string {
	switch {
	case s.Image != "":
		return s.Image
	case s.Origin == "":
		return ""
	}
	return s.Origin + "/" + iconFile
}

// each walks the writers, honouring --only and --skip, and hands every one
// that runs to do. Write and Validate differed only in what do was: produce
// the content, or read it back. Everything around that — the selection, the
// timing, the step, attributing findings to the writer — was written twice.
func each(rep *cli.Report, pick *picked, do func(Writer) (found []cli.Finding, covered, path string, err error)) int {
	ran := 0
	for _, w := range ordered() {
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
		// The validator reads the file and cannot know the page list was a
		// fallback rather than the site, so the writer says it.
		return append(found, onePage(w, s)...), covered, path, nil
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

// writeLlms is llms.txt: the site in the shape a language model reads it.
//
// The convention (llmstxt.org) is markdown, not a robots-style directive
// list: an H1 that names the site, a blockquote that says what it is in one
// sentence, then H2 sections of links. A model landing on a site has to crawl
// and guess otherwise, and what it guesses becomes what it tells people about
// you — which is the same problem a meta description solves for Search, one
// audience later.
//
// The same Site the other four writers read, so a repo that can write a
// sitemap can write this with no new input: the URLs are the pages, the title
// and description are the site's own words.
//
// The shape is cli.LLMsDoc's, not this file's. A command on this stack writes
// an llms.txt about itself — its verbs, from the one table its manual is
// rendered from, which is a thing only the command knows — and that document
// and this one are the same format about different subjects. Written out
// twice, the two would disagree about the format the first time either was
// edited; so the format is declared once and each side fills it in.
func writeLlms(s Site) (content, covered string, err error) {
	doc := cli.LLMsDoc{
		Title:   cli.Or(s.Title, hostOf(s.Origin)),
		Summary: s.Desc,
		Sections: []cli.LLMsSection{
			{Name: "Pages", Links: cli.Map(s.URLs, func(u string) cli.LLMsLink {
				return cli.LLMsLink{Text: pathOf(u), URL: u}
			})},
			// The sitemap is named rather than listed among the pages: a
			// model that wants the whole list should be told where it is, not
			// handed it twice.
			{Name: "Optional", Links: []cli.LLMsLink{{Text: SitemapFile,
				URL: s.Origin + "/sitemap.xml", Note: "every page, machine-readable"}}},
		},
	}
	return doc.String(), cli.Plural(len(s.URLs), "page") + " listed", nil
}

// pathOf is what to call a URL in a list of links: the last part of its path,
// or the host when it is the site's own front page.
func pathOf(u string) string {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Path == "" || parsed.Path == "/" {
		return cli.Or(parsed.Host, u)
	}
	return strings.Trim(parsed.Path, "/")
}

// hostOf is the site's name when nothing better was given.
func hostOf(origin string) string {
	if parsed, err := url.Parse(origin); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return origin
}

// SitemapFile is the one artifact other writers depend on: robots.txt names
// it and llms.txt points at it, so it appears in the registry entry that
// produces it and in the Needs of the two that do not. Spelling it four times
// is four chances for one of them to name a file nobody writes, which is the
// broken chain Needs exists to make impossible.
const SitemapFile = "sitemap.xml"

// producer is which writer makes an artifact, by the artifact's file name.
// One map, because both the ordering and the selection ask the same question.
func producer(name string) (Writer, bool) {
	for _, w := range writers {
		if w.Produces.Name == name {
			return w, true
		}
	}
	return Writer{}, false
}

// ordered is the writers in an order that respects Needs: nothing runs before
// what it depends on.
//
// Derived rather than kept, because the registry literal's order was the only
// thing making robots.txt's claim about sitemap.xml true, and a fact held by
// where a line sits in a slice is one an edit breaks silently. Registry order
// still decides between writers that do not depend on each other, so a report
// reads the same way it always did.
//
// A cycle is a programming error and a test names it; here it is broken by
// running the writer anyway, because a report that is wrong about one file
// beats a command that hangs.
func ordered() []Writer {
	var out []Writer
	done := map[string]bool{}
	var add func(w Writer, seen map[string]bool)
	add = func(w Writer, seen map[string]bool) {
		if done[w.Name] || seen[w.Name] {
			return
		}
		seen[w.Name] = true
		for _, need := range w.Needs {
			if dep, ok := producer(need); ok {
				add(dep, seen)
			}
		}
		done[w.Name] = true
		out = append(out, w)
	}
	for _, w := range writers {
		add(w, map[string]bool{})
	}
	return out
}

// chain adds to --only whatever the chosen writers depend on, so choosing a
// writer chooses the ones its output is about.
//
// `--only llms` asked for a file whose whole content is a claim about
// sitemap.xml; writing it alone produced an index pointing at a file that did
// not exist, and called it pass. Pulling the dependency in is what the person
// meant — they named the artifact they wanted, not the set of files they were
// willing to have written.
//
// --skip is the one place this refuses instead. Naming a writer in --only and
// its dependency in --skip is asking for both halves of a contradiction, and
// quietly honouring either one is worse than saying so.
func chain(c cli.Call, p *picked) error {
	if p == nil || len(p.only) == 0 {
		return nil
	}
	for again := true; again; {
		again = false
		for _, w := range ordered() {
			if !p.only[w.Name] {
				continue
			}
			for _, need := range w.Needs {
				dep, ok := producer(need)
				switch {
				case !ok:
					continue
				case p.skip[dep.Name]:
					return c.Usagef("--only %s wants %s, and only %s writes it — but --skip excludes %s",
						w.Name, need, dep.Name, dep.Name)
				case !p.only[dep.Name]:
					p.only[dep.Name] = true
					p.pulled = append(p.pulled, dep.Name+", which "+w.Name+" needs for "+need)
					again = true
				}
			}
		}
	}
	return nil
}

// repair writes the files that resolve what the checkers found.
//
// `check` already knew how — every finding carries FixedBy, the writer whose
// artifact resolves it — and stopped at telling you, printing the `dev seo
// write` line to run next. That is a loop a person closes by hand, and a loop
// a person closes by hand is one that stops being closed: the report named
// five fixable findings on this repo's own site and they stayed there.
//
// So it closes itself. The findings choose the writers, the chain pulls in
// what those writers depend on, and the files land in the directory the site
// is built from — deploy again and the next check is against what was fixed.
// Nothing here decides what is wrong; the checkers did that, and this only
// acts on it.
//
// Only what was found, deliberately. Writing every artifact would overwrite a
// file somebody edited to answer a finding that is no longer there, and the
// whole value of a fix that reads a report is that it touches what the report
// is about.
func repair(c cli.Call, rep *cli.Report) error {
	dir := c.Value("fix")
	if dir == "" {
		return nil
	}
	wanted := cli.Unique(cli.Collect(rep.Findings, func(f cli.Finding) (string, bool) {
		return f.FixedBy, f.FixedBy != ""
	}))
	if len(wanted) == 0 {
		fmt.Fprintf(c.Stderr, "nothing found that writing a file would fix\n")
		return nil
	}
	pick := &picked{only: cli.ToSet(wanted), skip: map[string]bool{}}
	if err := chain(c, pick); err != nil {
		return err
	}
	for _, why := range pick.pulled {
		fmt.Fprintf(c.Stderr, "also writing %s\n", why)
	}
	url := rep.Target
	urls, err := sitemapURLs(c, url)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.Stderr, "fixing %s in %s\n", cli.English(wanted), dir)
	return Write(c, dir, site(c, url, urls), rep, pick)
}
