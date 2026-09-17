package cli

import (
	"fmt"
	"regexp"
	"strings"
)

// A verb's usage is markdown, and it is read in two places that want
// different things: the rendered manual, where it is markdown among markdown,
// and a terminal, which has no renderer. Flatten is the second reading, so
// that one source serves both and neither can drift from the other.
//
// The shape is a markdown list: a verb is a list item whose first line is the
// signature and whose continuation is the description. That is not a
// convention invented here — it is what markdown already means by a list, so
// the grouping survives rendering. The terminal wants the description indented
// under its signature, and markdown cannot carry that indent itself: four
// spaces there would mean a code block, and two would be the list's own.
// So Flatten re-adds it, and CheckUsage holds usage to the subset Flatten
// knows.

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

// The constructs CheckUsage rejects. Each is either something Flatten would
// pass through as visible noise (a fence, a table, a link) or something it
// would silently mangle (a numbered or nested list, whose shape it does not
// read). Keeping the list closed is what stops Flatten growing into a
// markdown engine: a new construct is a decision, not a patch.
//
// bannedLines are whole-line shapes, matched on the line as written. They
// cannot be matched after inline code is blanked, because a fence is itself
// backticks: blanking would eat it and the check would pass.
var bannedLines = []struct {
	name string
	re   *regexp.Regexp
}{
	{"a code fence", regexp.MustCompile("^\\s*```")},
	{"a table", regexp.MustCompile(`^\s*\|`)},
	{"a numbered list", regexp.MustCompile(`^\s*\d+\.\s`)},
	{"a nested list", regexp.MustCompile(`^\s+[-*+]\s`)},
	{"a blockquote", regexp.MustCompile(`^\s*>`)},
	{"a setext heading", regexp.MustCompile(`^\s*(=+|-{2,})\s*$`)},
}

// bannedSpans are inline shapes, matched only outside inline code, so that
// what a code span protects is never mistaken for markup.
var bannedSpans = []struct {
	name string
	re   *regexp.Regexp
}{
	{"a link or image", regexp.MustCompile(`!?\[[^\]]*\]\([^)]*\)`)},
	{"emphasis (* and _ stay literal here, so globs stay globs)", regexp.MustCompile(`\*\*?[^*\s][^*]*\*\*?|(^|\s)_[^_]+_(\s|$)`)},
}

// angles finds <...> that would reach a markdown renderer as an HTML tag.
// `<app>.fly.dev` and `NAME<TAB>OWNER` are the real cases: unfenced, the
// renderer swallows the tag and the reader never sees it. In inline code it
// escapes correctly, so the rule is that every one of them wears backticks.
var angles = regexp.MustCompile(`<[A-Za-z/][^>\s]*>`)

// inlineCode is a `...` span, blanked before the other rules run so that what
// a span protects is never mistaken for markup.
var inlineCode = regexp.MustCompile("`[^`]*`")

// CheckUsage fails the test when a command's prose would not survive being
// read. A command's main_test.go calls it beside CheckSkill, so every repo on
// the stack holds its own prose to the same rules.
//
// The two are held to different rules, because they are read differently. A
// verb's usage is rendered twice — as markdown in the manual and as flattened
// text in a terminal — so it must stay inside the subset Flatten reads. Head
// and Tail only ever reach the manual, so fences, tables and emphasis are
// fine there and are not checked.
//
// What applies to both is the angle-bracket rule, because that is a markdown
// rendering bug rather than a flattening one, and prose is where it bit:
// tail.md shipped `NAME<TAB>OWNER` unfenced, so every rendered copy of the
// manual showed "NAMEOWNER" and lost the fact that the lines are
// tab-separated. Nothing caught it, which is why Head and Tail are checked
// here at all.
func CheckUsage(t TB, c Command) {
	t.Helper()
	verbs := c.all()
	for _, name := range sortedVerbs(verbs) {
		for _, problem := range usageProblems(verbs[name].Usage) {
			t.Errorf("%s %s usage: %s", c.Name, name, problem)
		}
	}
	for _, prose := range []struct{ what, md string }{{"head", c.Head}, {"tail", c.Tail}} {
		for _, problem := range angleProblems(blankFrontmatter(prose.md)) {
			t.Errorf("%s %s: %s", c.Name, prose.what, problem)
		}
	}
}

// usageProblems is every reason a usage string is outside the subset, as
// messages naming the line and its fix.
func usageProblems(md string) []string {
	var out []string
	for i, line := range strings.Split(strings.TrimRight(md, "\n"), "\n") {
		for _, b := range bannedLines {
			if b.re.MatchString(line) {
				out = append(out, fmt.Sprintf("line %d uses %s, which the terminal rendering does not read: %q", i+1, b.name, line))
			}
		}
		bare := inlineCode.ReplaceAllString(line, "")
		for _, b := range bannedSpans {
			if b.re.MatchString(bare) {
				out = append(out, fmt.Sprintf("line %d uses %s, which the terminal rendering does not read: %q", i+1, b.name, line))
			}
		}
	}
	return append(out, angleProblems(md)...)
}

// angleProblems is every <...> a markdown renderer would eat, as messages
// naming the line and its fix.
func angleProblems(md string) []string {
	var out []string
	for i, line := range strings.Split(strings.TrimRight(md, "\n"), "\n") {
		bare := inlineCode.ReplaceAllString(line, "")
		for _, m := range angles.FindAllString(bare, -1) {
			out = append(out, fmt.Sprintf("line %d has %s outside inline code; a markdown renderer eats it as an HTML tag, so write it as `%s`: %q", i+1, m, m, line))
		}
	}
	return out
}

// blankFrontmatter empties a leading --- block, which is YAML the renderer
// never sees, keeping the lines so that a message's line number still counts
// from the top of the file a person edits.
func blankFrontmatter(md string) string {
	lines := strings.Split(md, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return md
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			for j := 0; j <= i; j++ {
				lines[j] = ""
			}
			break
		}
	}
	return strings.Join(lines, "\n")
}
