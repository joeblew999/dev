// Package checkers is every tool `dev seo check` runs, one file each.
//
// A package of its own, below the verb rather than beside it, because the
// verb is about what Google asks of a page and these are about what one
// program prints. Adding a tool is a file here and a line in All, and
// nothing above it changes.
//
// No checker is made to share a shape: each says what it found in its own
// JSON or its own prose, and what this package takes is only the part they
// all have — an issue, where it is, how to fix it. Nothing a checker said is
// rewritten, either: a finding keeps the id the checker emitted, because a CI
// rule matches on that and a report that translates a tool is a report you
// cannot check against the tool.
package checkers

import (
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
)

// Checker is one tool. Everything that knows a tool exists is one of these
// and the Read that goes with it.
type Checker struct {
	Name     string                               // the binary, and what it is called in the report
	Pin      string                               // the mise.toml [tools] line that installs it
	Provides string                               // what running it gets you, said in the report when it does not
	Cost     string                               // roughly how long it takes
	Args     func(a Ask) []string                 // how to ask it
	Read     func(res tool.Result) (Found, error) // what it said, in the part every checker shares

	// Fetch is a path under the site to download before running, for a tool
	// that takes a file rather than a URL — Google's robots matcher wants the
	// rules on disk. The file lands in Ask.File and is removed afterwards.
	Fetch string
}

// Ask is what a checker is being asked to look at.
type Ask struct {
	URL   string // the page or site
	Pages int    // how far to crawl, when the tool crawls
	File  string // what Fetch downloaded, when the checker asked for one
}

// Found is the part of a checker's answer every checker has: what it covered,
// and what it wants fixed. Whatever else it said stays in its sub-report.
type Found struct {
	Pages  int
	Links  int
	Broken int
	Issues []cli.Finding

	// Summary is what this checker covered, in its own words, for one whose
	// counts do not describe it. muffet reports only the pages that have
	// something wrong, so with a clean site it has no counts at all — and
	// "nothing to report" beside an empty file reads as a failure when it
	// means the opposite.
	Summary string
}

// Covered is what a checker looked at, in the words that apply to it: a
// crawler counts pages, a link checker counts what it found broken, and one
// that reports neither says so rather than printing two zeroes.
func (f Found) Covered() string {
	if f.Summary != "" {
		return f.Summary
	}
	var parts []string
	for _, p := range []struct {
		n    int
		word string
	}{{f.Pages, "page"}, {f.Links, "link"}, {f.Broken, "broken link"}} {
		switch {
		case p.n == 1:
			parts = append(parts, "1 "+p.word)
		case p.n > 1:
			parts = append(parts, plural(p.n, p.word))
		}
	}
	if len(parts) == 0 {
		return "nothing to report"
	}
	return strings.Join(parts, ", ")
}

// Paged appends a page limit in the spelling both crawlers use, for a
// checker that takes one and skips it at zero.
func Paged(args []string, pages int) []string {
	if pages <= 0 {
		return args
	}
	return append(args, "--max-pages", itoa(pages))
}

// All is the registry, in the order a report lists them — whatever order they
// finished in.
var All = []Checker{kitsune, scoutly, scry, muffet, seoAudit, ldlint, robotsRules}

// JSON builds a Read for a checker that answers in JSON: decode into T, then
// say what was found. Every one of them had written the same decode and the
// same error check around the only part that differs, which is the shape and
// what it means.
//
// A generic function rather than a method, because the type is the whole of
// what varies and the receiver would be nothing.
func JSON[T any](what string, found func(T) Found) func(tool.Result) (Found, error) {
	return func(res tool.Result) (Found, error) {
		v, err := res.JSON[T](what)
		if err != nil {
			return Found{}, err
		}
		return found(v), nil
	}
}

// Text builds a Read for a checker that answers in prose: every line is
// offered to each rule in turn, and the first that matches wins, so order is
// precedence. Two checkers here print sentences rather than JSON, and both
// had written the same loop around a different switch.
func Text(rules ...Rule) func(tool.Result) (Found, error) {
	return func(res tool.Result) (Found, error) {
		var found Found
		for _, line := range cli.Lines(res.Out) {
			line = strings.TrimSpace(line)
			for _, r := range rules {
				if !r.When(line) {
					continue
				}
				if r.Then != nil {
					r.Then(line, &found)
				}
				break
			}
		}
		return found, nil
	}
}

// Rule is one thing to look for in a line, and what it means.
type Rule struct {
	When func(line string) bool
	Then func(line string, found *Found)
}

// Has and Ends are the two shapes a line is matched by here. Match by
// pattern, never by position: one tool prints its verdict and then a notice
// explaining it, and reading the last line called that no verdict at all.
func Has(sub string) func(string) bool {
	return func(l string) bool { return strings.Contains(l, sub) }
}

func Ends(suffix string) func(string) bool {
	return func(l string) bool { return strings.HasSuffix(l, suffix) }
}

// Issue is the finding shape every checker here produces, so the four fields
// that are the same every time are written once.
func Issue(toolName, id, severity, message, where, fix string) cli.Finding {
	return cli.Finding{
		Tool: toolName, ID: id, Severity: severity,
		Message: message, Where: where, Fix: fix,
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return itoa(n) + " " + word + "s"
}

// itoa keeps this file free of fmt for one number.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
