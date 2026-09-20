// The site dev serves about itself.
//
// Every page here is markdown this repo already ships: README.md, and the
// manuals under skills/ that `dev skill` renders from the verbs. So the site
// has no content of its own and cannot drift from the command — change a verb
// and the page changes with it, through the same render that feeds the
// terminal index and llms.txt.
//
// What is on which page is the part that had to be decided, and the first
// answer was wrong. The front page was the dev skill: a briefing written for
// an agent that already has the tool installed and the repo open. It opened
// "A repo on this stack is a few commands", which presumes you have adopted
// the stack; it gave fourteen imperatives, which is right for an agent under
// instruction and hectoring to a stranger; and in 27KB it never once printed
// the line that installs dev, which the tool knows and prints itself. A page
// a visitor cannot act on is the worst failure a front page has.
//
// So the front page is README.md, which was already the right document and
// was already generated where it mattered: the install block between the two
// <!-- pin --> markers is written by `dev skill` from the key the releases
// are signed with, so the one instruction a reader follows before they have
// the tool cannot name a key that no longer signs.
//
// And the manuals are split, a page per section. They were already written
// that way — a verb group carries its own prose and reading one has never
// meant reading the one above it — and one scroll of fifteen sections was
// also what made `perf.dom_size.metrics` fire: 90 children of <body> against
// a limit of 60. That checker was right and it was dismissed once here. A
// document is not a page.
//
// A generator rather than a Worker script, because the pages never change
// between deploys: Cloudflare serves them as static assets, which costs no
// invocation and no cold start, and the Worker directory holds only what
// wrangler needs to upload them.
package main

import (
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// Search's own limits, and every one of the four has bitten this site.
//
// A title under 30 characters is reported as too short — /cli/ shipped at 29
// and was faulted the same day — and one over 60 is truncated in the result,
// so the end of it never reaches a reader. A description under 50 is reported
// as badly sized, and one over 160 is cut off mid-word.
//
// They are here rather than left to the live checker because a generator that
// can emit a page Search will fault should not be able to. The build fails
// instead, seconds after the edit, rather than a checker saying so after a
// deploy.
const (
	titleMin, titleMax = 30, 60
	descMin, descMax   = 50, 160
)

// source is a markdown file this repo already ships, and the pages it becomes.
//
// There is no content written for the site and there is not going to be: the
// moment a page is authored here it is a second copy of something, and the
// second copy is the one that goes stale.
type source struct {
	File string // the markdown this repo already ships
	Base string // the URL prefix its pages live under
	Nav  string // what to call it in the navigation every page carries
	H1   string // the heading of the page at Base
	// Title and Desc are that page's own, because the file's first heading
	// is a word ("dev") and Search wants a sentence.
	Title string
	Desc  string
	// Suffix is what a section's title says after the section's own name,
	// which is how a one-word heading reaches thirty characters. Dropped
	// when the heading is long enough to leave no room for it.
	Suffix string
	// Split says this is a manual to cut into a page per section. A README is
	// one page and stays one page.
	Split bool
}

var sources = []source{
	{
		File: "README.md", Base: "/", Nav: "dev", H1: "dev",
		Title: "dev — the stack's developer tool",
		Desc:  "The developer tool of the mise + fnox + hk + packslip stack: what it is, the line that installs it, and where its manual lives.",
	},
	{
		File: "skills/dev/SKILL.md", Base: "/dev/", Nav: "The dev manual",
		H1:     "The dev manual",
		Title:  "The dev manual — every verb, a group at a time",
		Desc:   "Build, check, run, release and deploy a repo on this stack. Every verb dev has, a page per group, rendered from the verbs themselves.",
		Suffix: "dev, the stack's developer tool",
		Split:  true,
	},
	{
		File: "skills/cli/SKILL.md", Base: "/cli/", Nav: "Writing a command",
		H1:     "The cli manual",
		Title:  "The cli manual — writing a command on this stack",
		Desc:   "The verb system every command on this stack is built on: what a verb declares, the helpers it is given, and the tests that hold a manual to its code.",
		Suffix: "cli, the stack's verb system",
		Split:  true,
	},
}

// page is one rendered page of the site.
type page struct {
	Path  string // the URL it is served at
	Group string // the Base of the source it came from, for the section list
	H1    string
	Title string
	Desc  string
	Body  string // markdown, not yet rendered
}

func main() {
	out := flag.String("out", "site/public", "where to write the site")
	origin := flag.String("origin", "", "the site's own origin, for canonical links")
	urls := flag.String("urls", "site/urls.txt", "where to write the page list the sitemap is built from")
	flag.Parse()
	if err := build(*out, *origin, *urls); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func build(out, origin, urls string) error {
	var pages []page
	for _, s := range sources {
		got, err := s.pages()
		if err != nil {
			return err
		}
		pages = append(pages, got...)
	}
	if err := searchable(pages); err != nil {
		return err
	}
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	for _, p := range pages {
		var body strings.Builder
		if err := md.Convert([]byte(p.Body), &body); err != nil {
			return err
		}
		dir := filepath.Join(out, filepath.FromSlash(strings.Trim(p.Path, "/")))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "index.html"),
			[]byte(document(p, origin, body.String(), pages)), 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "  wrote %d pages from %d markdown files\n", len(pages), len(sources))
	return pageList(urls, origin, pages)
}

// pageList writes the URLs the sitemap, llms.txt and the checkers are all
// built from.
//
// Written rather than kept by hand, because it is the same fact as the page
// list this just rendered and a hand-kept copy of a generated fact is a copy
// that is wrong the first time a section is renamed. Everything downstream
// reads this one file: `dev seo write --urls` builds the sitemap from it, and
// `dev seo check` is pointed at the site it describes.
func pageList(path, origin string, pages []page) error {
	if path == "" {
		return nil
	}
	var b strings.Builder
	b.WriteString("# Written by site/gen. Every page of the site, in the order it\n")
	b.WriteString("# renders them; the sitemap and llms.txt are built from this.\n")
	for _, p := range pages {
		b.WriteString(origin + p.Path + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "  wrote %-28s %d URLs\n", path, len(pages))
	return nil
}

// searchable holds every page to the ranges Search works in, and names all of
// them at once rather than failing on the first.
//
// A generator that can emit a page a checker will fault is a generator that
// will, and the loop that finds out is check, deploy, crawl — minutes, after
// the fact, against a live site. This is the same knowledge a second earlier.
func searchable(pages []page) error {
	var bad []string
	for _, p := range pages {
		if n := utf8.RuneCountInString(p.Title); n < titleMin || n > titleMax {
			bad = append(bad, fmt.Sprintf("%s: title is %d characters, want %d-%d: %q",
				p.Path, n, titleMin, titleMax, p.Title))
		}
		if n := utf8.RuneCountInString(p.Desc); n < descMin || n > descMax {
			bad = append(bad, fmt.Sprintf("%s: description is %d characters, want %d-%d: %q",
				p.Path, n, descMin, descMax, p.Desc))
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("%d page(s) Search would fault:\n  %s", len(bad), strings.Join(bad, "\n  "))
}

// pages cuts one markdown file into the pages it is already written as.
func (s source) pages() ([]page, error) {
	data, err := os.ReadFile(s.File)
	if err != nil {
		return nil, err
	}
	md := headless(s.relink(uncomment(frontmatterless(string(data)))))
	intro, secs := split(md)
	index := page{Path: s.Base, Group: s.Base, H1: s.H1, Title: s.Title, Desc: s.Desc, Body: intro}
	if !s.Split {
		// One page, so it keeps everything: a README's ## Get it is the whole
		// reason the page exists.
		index.Body = md
		return []page{index}, nil
	}
	out := []page{index}
	for _, sec := range secs {
		out = append(out, page{
			Path:  s.Base + slug(sec.head) + "/",
			Group: s.Base,
			H1:    sec.head,
			Title: fit(sec.head, s.Suffix),
			Desc:  describe(sec.body, s.Desc),
			Body:  sec.body,
		})
	}
	return out, nil
}

// relink points a link to a file this site publishes at the page that
// publishes it. README.md links to skills/dev/SKILL.md, which is a path in
// the repository and a 404 on the web — the kind of thing the link checker
// reports and a reader meets first.
func (s source) relink(md string) string {
	for _, other := range sources {
		md = strings.ReplaceAll(md, "]("+other.File+")", "]("+other.Base+")")
	}
	return md
}

// section is one page's worth of a manual: its heading, and the markdown
// under it.
type section struct{ head, body string }

// split cuts a manual at its headings, which is where it was already cut.
//
// One rule, no exceptions and nothing to decide per file: a level-2 or
// level-3 heading starts a page. That gives exactly what reading the manual
// suggests — a level-2 with level-3 sections under it becomes the page that
// introduces them, and each of those becomes a page of its own — without the
// rule having to say so.
//
// Fenced code is walked over rather than scanned, because a shell session in
// a manual is full of lines that start with #.
func split(md string) (intro string, secs []section) {
	var head string
	var buf []string
	fenced := false
	close := func() {
		body := strings.TrimSpace(strings.Join(buf, "\n"))
		switch {
		case head == "":
			intro = body
		case body != "" || len(secs) > 0:
			secs = append(secs, section{head: head, body: body})
		}
		buf = nil
	}
	for line := range strings.SplitSeq(md, "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}
		if !fenced && (strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ")) {
			close()
			head = strings.TrimSpace(strings.TrimLeft(line, "#"))
			continue
		}
		buf = append(buf, line)
	}
	close()
	return intro, secs
}

// headless drops the document's own H1. Every page here renders its own, from
// what the site calls that page rather than from what the file calls itself —
// and two H1s on one page is a finding.
//
// It runs before the split rather than on the one-page branch only, which is
// where it was and what that cost: the level-1 heading sits above the first
// level-2, so it landed in the intro, and both manual index pages shipped
// with two H1s. Three checkers reported it within the minute.
func headless(md string) string {
	var out []string
	fenced := false
	for line := range strings.SplitSeq(md, "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}
		if !fenced && strings.HasPrefix(line, "# ") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// frontmatterless drops the YAML block a skill carries for the agent that
// reads it. It is metadata for a loader, not the first paragraph of a page.
func frontmatterless(text string) string {
	if !strings.HasPrefix(text, "---\n") {
		return text
	}
	if _, rest, ok := strings.Cut(text[4:], "\n---\n"); ok {
		return rest
	}
	return text
}

// uncomment drops HTML comments, which markdown passes straight through.
//
// They are instructions to whoever edits the file — "Generated by dev skill,
// never edit this file", and the two markers a README puts around its install
// block — and a reader of the website is not that person.
func uncomment(md string) string {
	for {
		open := strings.Index(md, "<!--")
		if open < 0 {
			return md
		}
		shut := strings.Index(md[open:], "-->")
		if shut < 0 {
			return md[:open]
		}
		md = md[:open] + md[open+shut+len("-->"):]
	}
}

// slug is the path a heading is served at.
func slug(head string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(head) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		case r == '\'' || r == '’':
			// An apostrophe joins a word rather than breaking it, so
			// "A command's manual" is a-commands-manual and not a-command-s.
		default:
			dash = true
		}
	}
	return b.String()
}

// fit is a <title> Search will show whole.
//
// The section's own heading is the true part and comes first; the site's name
// follows it when there is room, which is what carries a two-word heading
// past thirty characters. A heading long enough to leave no room keeps the
// whole title to itself.
func fit(head, suffix string) string {
	if full := head + " — " + suffix; utf8.RuneCountInString(full) <= titleMax {
		return full
	}
	if utf8.RuneCountInString(head) >= titleMin {
		return clip(head, titleMax)
	}
	return clip(head+" — "+suffix, titleMax)
}

// describe is the sentence Search shows under the title: the section's own
// opening prose, which was written by whoever knew what the section is about.
//
// A section that opens with a table or a code block has no such prose, and
// one that opens with a single short line has not enough of it, so the site's
// own description finishes the sentence rather than leaving it short enough
// to be faulted.
func describe(body, fallback string) string {
	text := prose(body)
	if utf8.RuneCountInString(text) < descMin {
		text = strings.TrimSpace(text + " " + fallback)
	}
	return clip(text, descMax)
}

// prose is a section's words with its markup taken off.
//
// Code and tables are dropped rather than flattened: a description that opens
// halfway through a shell command or a row of pipes tells a reader nothing,
// and it is the first thing they see of the page in a result.
func prose(md string) string {
	var out []string
	fenced := false
	for line := range strings.SplitSeq(md, "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		line = strings.TrimSpace(line)
		if fenced || line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") {
			continue
		}
		out = append(out, strings.TrimSpace(strings.TrimLeft(line, "-*>+ ")))
	}
	return strings.Join(strings.Fields(
		strings.NewReplacer("`", "", "**", "", "<br>", " ").Replace(strings.Join(out, " "))), " ")
}

// clip cuts to a length at a word boundary, because a description cut
// mid-word reads as a bug in the site rather than as a limit of the result.
func clip(text string, max int) string {
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	runes := []rune(text)
	cut := string(runes[:max-1])
	if at := strings.LastIndexByte(cut, ' '); at > 0 {
		cut = cut[:at]
	}
	return strings.TrimRight(cut, " ,;:—-") + "…"
}

// document is the page around the rendered markdown: the tags Search and a
// link preview read, the navigation, and nothing a reader has to download.
//
// The CSS is inline, and it stays inline on measurement rather than on
// taste. Linking it would let the Content-Security-Policy drop
// 'unsafe-inline' from style-src, which two checkers report — and it would
// trade those for `perf.render_blocking.styles`, a third checker's warning
// about exactly the round trip this site does not need. The answer that costs
// nothing is a policy that names this block's sha256, which is a change to
// the policy and not to the page.
func document(p page, origin, body string, all []page) string {
	canonical := origin + p.Path
	return `<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + html.EscapeString(p.Title) + `</title>
<meta name="description" content="` + html.EscapeString(p.Desc) + `">
<link rel="canonical" href="` + html.EscapeString(canonical) + `">
<link rel="icon" href="/icon.png">
<link rel="apple-touch-icon" href="/icon.png">
<meta property="og:type" content="website">
<meta property="og:title" content="` + html.EscapeString(p.Title) + `">
<meta property="og:description" content="` + html.EscapeString(p.Desc) + `">
<meta property="og:url" content="` + html.EscapeString(canonical) + `">
<meta property="og:image" content="` + html.EscapeString(origin) + `/icon.png">
<meta name="twitter:card" content="summary">
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"TechArticle","url":"` + canonical + `","name":"` + p.Title + `","description":"` + p.Desc + `"}
</script>
<style>
:root{--ink:#111;--dim:#555;--line:#e3e3e3;--bg:#fff;--code:#f6f6f6}
@media(prefers-color-scheme:dark){:root{--ink:#e8e8e8;--dim:#a0a0a0;--line:#2c2c2c;--bg:#111;--code:#1c1c1c}}
*{box-sizing:border-box}
body{max-width:46rem;margin:0 auto;padding:2rem 1rem 6rem;background:var(--bg);color:var(--ink);
  font:16px/1.65 ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}
nav.top{display:flex;flex-wrap:wrap;gap:1.25rem;padding-bottom:1.5rem;margin-bottom:2rem;border-bottom:1px solid var(--line)}
nav a{color:var(--dim);text-decoration:none}
nav.top a{font-weight:600}
nav a:hover,nav a[aria-current]{color:var(--ink)}
nav.sections{display:flex;flex-direction:column;gap:.4rem;margin-top:4rem;padding-top:1.5rem;border-top:1px solid var(--line)}
nav.sections b{color:var(--ink);font-size:.9rem;margin-bottom:.35rem}
nav.sections a[aria-current]{font-weight:600}
h1,h2,h3{line-height:1.25;margin:2.5rem 0 1rem}
h1{margin-top:0;font-size:2rem}
h2{font-size:1.4rem;padding-top:1rem;border-top:1px solid var(--line)}
h3{font-size:1.1rem}
a{color:inherit}
code{background:var(--code);padding:.15em .35em;border-radius:3px;font-size:.9em}
pre{background:var(--code);padding:1rem;border-radius:6px;overflow-x:auto}
pre code{background:none;padding:0}
table{border-collapse:collapse;width:100%;margin:1.5rem 0;display:block;overflow-x:auto}
th,td{border:1px solid var(--line);padding:.5rem .75rem;text-align:left;vertical-align:top}
blockquote{margin:1.5rem 0;padding-left:1rem;border-left:3px solid var(--line);color:var(--dim)}
</style>
` + topNav(p) + `
<h1>` + html.EscapeString(p.H1) + `</h1>
` + body + sectionNav(p, all) + `
</html>
`
}

// topNav is the same three links on every page: what dev is, and the two
// manuals.
func topNav(p page) string {
	var b strings.Builder
	b.WriteString(`<nav class="top">`)
	for _, s := range sources {
		fmt.Fprintf(&b, `<a href=%q%s>%s</a>`, s.Base, current(p.Path == s.Base), html.EscapeString(s.Nav))
	}
	b.WriteString("</nav>")
	return b.String()
}

// sectionNav is the rest of the manual this page belongs to.
//
// It is what a split manual owes a reader: fifteen sections on one scroll
// were at least all in front of you, and fifteen pages with no list of each
// other are worse than the scroll was. In a <nav> so that the word count a
// checker takes off the page is the page's own.
func sectionNav(p page, all []page) string {
	var b strings.Builder
	for _, other := range all {
		if other.Group != p.Group || other.Path == p.Group {
			continue
		}
		if b.Len() == 0 {
			b.WriteString(`<nav class="sections"><b>The rest of this manual</b>`)
		}
		fmt.Fprintf(&b, `<a href=%q%s>%s</a>`,
			other.Path, current(other.Path == p.Path), html.EscapeString(other.H1))
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String() + "</nav>"
}

// current marks the page you are on, which is the one thing a list of links
// cannot say by being a list of links.
func current(is bool) string {
	if is {
		return ` aria-current="page"`
	}
	return ""
}
