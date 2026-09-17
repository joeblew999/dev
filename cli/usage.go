package cli

import (
	"strings"
)

// Flatten renders markdown usage as the plain text a terminal shows: headings
// lose their #, list items lose their marker, continuations gain the four
// spaces that mark them as belonging to the signature above, and inline code
// loses its backticks.
//
// It never strips * or _, because usage text says things like `**/*.go` and
// `secrets:*`; emphasis is banned by CheckUsage rather than unwound here, so
// that a glob can never be mistaken for markup. Every line is right-trimmed:
// the rendered manual is committed, and hk's trailing_whitespace check reads
// it like any other file.
func Flatten(md string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(md, "\n"), "\n") {
		switch {
		case strings.TrimSpace(line) == "":
			b.WriteString("\n")
		case strings.HasPrefix(line, "#"):
			b.WriteString(unmark(strings.TrimLeft(line, "# ")) + "\n")
		case strings.HasPrefix(line, "- "):
			b.WriteString(unmark(line[2:]) + "\n")
		case strings.HasPrefix(line, "  "):
			b.WriteString(indent(unmark(strings.TrimSpace(line))) + "\n")
		default:
			b.WriteString(unmark(line) + "\n")
		}
	}
	return b.String()
}

// usageIndent is what a description is indented by under its signature: the
// width the hand-written usage text used before it was markdown.
const usageIndent = "    "

// indent puts a continuation under its signature, unless flattening left the
// line empty — an indent alone would be trailing whitespace.
func indent(s string) string {
	if s == "" {
		return ""
	}
	return usageIndent + s
}

// unmark drops the markup that means nothing in a terminal, and right-trims
// what is left.
func unmark(s string) string {
	return strings.TrimRight(strings.ReplaceAll(s, "`", ""), " \t")
}

// Verbs renders the verb list for a group: one signature per verb, each with
// its own one-line Desc under it. Nothing here is written by hand — the
// signature comes from Args and Flags, the line from Desc — so a usage.md
// carries only the prose that explains why the group exists.
//
// paths are the verbs this group covers, in the order the manual reads them.
func Verbs(name string, verbs map[string]Verb, paths []string) string {
	var b strings.Builder
	for _, path := range paths {
		v, _, ok := lookup(verbs, path)
		if !ok {
			continue
		}
		if len(v.Subs) > 0 {
			for _, sub := range sortedVerbs(v.Subs) {
				b.WriteString(one(name, verbs, path+" "+sub))
			}
			continue
		}
		b.WriteString(one(name, verbs, path))
	}
	return b.String()
}

// one is a single verb as the manual shows it: its signature, then its line.
func one(name string, verbs map[string]Verb, path string) string {
	v, _, ok := lookup(verbs, path)
	if !ok {
		return ""
	}
	out := "- `" + v.Signature(name, path) + "`\n"
	if v.Desc != "" {
		out += "  " + v.Desc + "\n"
	}
	return out
}

// lookup finds the verb a path names, following Subs for "secrets push", and
// returns it with the path as the signature should print it.
func lookup(verbs map[string]Verb, path string) (Verb, string, bool) {
	parts := strings.Fields(path)
	if len(parts) == 0 {
		return Verb{}, "", false
	}
	v, ok := verbs[parts[0]]
	if !ok {
		return Verb{}, "", false
	}
	for _, p := range parts[1:] {
		sub, ok := v.Subs[p]
		if !ok {
			return Verb{}, "", false
		}
		v = sub
	}
	return v, path, true
}
