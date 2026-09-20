// The site dev serves about itself.
//
// Every page here is a manual this repo already ships: skills/<name>/SKILL.md,
// rendered from the verbs by `dev skill`. So the site has no content of its
// own and cannot drift from the command — change a verb and the page changes
// with it, through the same render that feeds the terminal index and llms.txt.
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

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// page is one manual, and where it lands on the site.
type page struct {
	Source string // the markdown this repo already ships
	Path   string // the URL path it is served at
	Title  string
	Desc   string
}

func main() {
	out := flag.String("out", "site/public", "where to write the site")
	origin := flag.String("origin", "", "the site's own origin, for canonical links")
	flag.Parse()
	if err := build(*out, *origin); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func build(out, origin string) error {
	pages := []page{
		{Source: "skills/dev/SKILL.md", Path: "/",
			Title: "dev — the stack's developer tool",
			Desc:  "Build, check, run, release and deploy the commands of a repo on the mise + fnox + hk + packslip stack."},
		{Source: "skills/cli/SKILL.md", Path: "/cli/",
			Title: "cli — the stack's verb system",
			Desc:  "Write a command on this stack's verb system: the API, what a verb declares, and the test helpers that hold a manual to its code."},
	}
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	for _, p := range pages {
		source, err := os.ReadFile(p.Source)
		if err != nil {
			return err
		}
		var body strings.Builder
		if err := md.Convert(frontmatterless(source), &body); err != nil {
			return err
		}
		dir := filepath.Join(out, filepath.FromSlash(strings.Trim(p.Path, "/")))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		file := filepath.Join(dir, "index.html")
		if err := os.WriteFile(file, []byte(document(p, origin, body.String(), pages)), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "  wrote %-28s from %s\n", file, p.Source)
	}
	return nil
}

// frontmatterless drops the YAML block a skill carries for the agent that
// reads it. It is metadata for a loader, not the first paragraph of a page.
func frontmatterless(source []byte) []byte {
	text := string(source)
	if !strings.HasPrefix(text, "---\n") {
		return source
	}
	if _, rest, ok := strings.Cut(text[4:], "\n---\n"); ok {
		return []byte(rest)
	}
	return source
}

// document is the page around the rendered markdown: the tags Search and a
// link preview read, and nothing a reader has to download.
//
// The CSS is inline and short on purpose. A stylesheet is a second request
// before the page can be painted, and this site is text — the whole of what
// it needs is a measure, a readable size and code that does not run off the
// side.
func document(p page, origin, body string, all []page) string {
	canonical := origin + p.Path
	var nav strings.Builder
	for _, other := range all {
		fmt.Fprintf(&nav, `<a href=%q>%s</a>`, other.Path, html.EscapeString(strings.SplitN(other.Title, " ", 2)[0]))
	}
	return `<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + html.EscapeString(p.Title) + `</title>
<meta name="description" content="` + html.EscapeString(p.Desc) + `">
<link rel="canonical" href="` + html.EscapeString(canonical) + `">
<meta property="og:type" content="website">
<meta property="og:title" content="` + html.EscapeString(p.Title) + `">
<meta property="og:description" content="` + html.EscapeString(p.Desc) + `">
<meta property="og:url" content="` + html.EscapeString(canonical) + `">
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
nav{display:flex;gap:1.25rem;padding-bottom:1.5rem;margin-bottom:2rem;border-bottom:1px solid var(--line)}
nav a{color:var(--dim);text-decoration:none;font-weight:600}
nav a:hover{color:var(--ink)}
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
<nav>` + nav.String() + `</nav>
` + body + `
</html>
`
}
