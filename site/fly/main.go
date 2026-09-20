// The same site, served from Fly.
//
// Cloudflare serves site/public as static assets and reads `_headers` itself.
// Fly runs a container, so this is what reads it there: the same file dev
// writes, applied by the same rules, so a header is declared once and two
// clouds honour it. The alternative was a second copy of the header set in a
// fly.toml, which is the fact-in-two-places this stack keeps deleting.
//
// The pages are embedded rather than mounted, so the image is the site: there
// is no volume to keep in step and a rollback rolls the content back with it.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// public is written by site/gen. Embedding reaches only downward, which is
// why the generator writes a copy here rather than this pointing at a sibling.
//
//go:embed all:public
var public embed.FS

func main() {
	site, err := fs.Sub(public, "public")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	rules := headerRules(site)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Fprintf(os.Stderr, "serving on :%s with %d header rules\n", port, len(rules))
	if err := http.ListenAndServe(":"+port, serve(site, rules)); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// rule is one path pattern from `_headers` and what it sets.
type rule struct {
	pattern string
	headers [][2]string
}

// serve answers a request the way Cloudflare's asset server does: the headers
// `_headers` declares for that path, then the file, with a directory served
// by its index.html so /dev/ works and /dev/index.html is not the URL anyone
// sees.
func serve(site fs.FS, rules []rule) http.Handler {
	files := http.FileServer(http.FS(site))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, ru := range rules {
			if matches(ru.pattern, r.URL.Path) {
				for _, h := range ru.headers {
					w.Header().Set(h[0], h[1])
				}
			}
		}
		files.ServeHTTP(w, r)
	})
}

// matches is Cloudflare's `_headers` path matching, which is the only reason
// to write it out: /* is everything, /*/ is any directory, /*.html is any
// page, and a plain path is itself. Later rules win by being applied last,
// the same as there.
func matches(pattern, url string) bool {
	switch {
	case pattern == "/*":
		return true
	case pattern == "/*/":
		return strings.HasSuffix(url, "/") && url != "/"
	case strings.HasPrefix(pattern, "/*."):
		return strings.HasSuffix(url, strings.TrimPrefix(pattern, "/*"))
	default:
		return url == pattern
	}
}

// headerRules reads `_headers` out of the site itself. A site without one
// gets none, which is the same thing Cloudflare does with it.
func headerRules(site fs.FS) []rule {
	data, err := fs.ReadFile(site, "_headers")
	if err != nil {
		return nil
	}
	var rules []rule
	for line := range strings.Lines(string(data)) {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
			continue
		// A rule begins at column zero; its headers are indented under it.
		case !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t"):
			rules = append(rules, rule{pattern: path.Clean("/" + strings.TrimPrefix(trimmed, "/"))})
			if strings.HasSuffix(trimmed, "/") && trimmed != "/" {
				rules[len(rules)-1].pattern = trimmed
			}
		case len(rules) > 0:
			if name, value, ok := strings.Cut(trimmed, ":"); ok {
				rules[len(rules)-1].headers = append(rules[len(rules)-1].headers,
					[2]string{strings.TrimSpace(name), strings.TrimSpace(value)})
			}
		}
	}
	return rules
}
