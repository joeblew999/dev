// What a test holds a command to. Every check here answers one question a
// person would otherwise have to ask by reading two things side by side: is
// this copy current, can this markdown be printed with no renderer, does this
// verb say what it is for.
package cli

import (
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// TB is the part of testing.TB that CheckSkill needs, so that importing this
// package never links the testing package into a binary.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
}

// CheckSkill fails the test when any copy of c's manual differs from what
// its verbs render. A command's main_test.go calls it, so `go test` — and so
// `dev check` — holds every manual to its verbs.
func CheckSkill(t TB, c Command) {
	t.Helper()
	want := c.render()
	shipped, claude, agents, err := c.paths()
	if err != nil {
		t.Errorf("%v", err)
		return
	}
	for _, p := range []string{shipped, claude, agents} {
		have, err := os.ReadFile(p)
		if err != nil || string(have) != want {
			t.Errorf("%s is stale; regenerate it with: %s skill", rel(p), c.Name)
		}
	}
	dir, err := root(".")
	if err != nil {
		return
	}
	for name, body := range c.Skills {
		want := withProvenance(body, c.Name)
		for _, p := range skillPaths(dir, name) {
			have, err := os.ReadFile(p)
			if err != nil || string(have) != want {
				t.Errorf("%s is stale; regenerate it with: %s skill", rel(p), c.Name)
			}
		}
	}
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
// text in a terminal — so it must stay inside the subset Flatten reads. The
// skill prose only ever reaches the manual, so fences, tables and emphasis
// are fine there and are not checked.
//
// What applies to both is the angle-bracket rule, because that is a markdown
// rendering bug rather than a flattening one, and prose is where it bit: the
// manual once shipped `NAME<TAB>OWNER` unfenced, so every rendered copy showed
// "NAMEOWNER" and lost the fact that the lines are tab-separated. Nothing
// caught it, which is why the prose is checked here at all.
func CheckUsage(t TB, c Command) {
	t.Helper()
	verbs := c.all()
	for _, name := range sortedVerbs(verbs) {
		for _, problem := range usageProblems(verbs[name].Usage) {
			t.Errorf("%s %s usage: %s", c.Name, name, problem)
		}
	}
	for _, problem := range angleProblems(blankFrontmatter(c.Skill)) {
		t.Errorf("%s skill.md: %s", c.Name, problem)
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
			clear(lines[:i+1])
			break
		}
	}
	return strings.Join(lines, "\n")
}

// CheckDescribed fails when a verb says nothing about itself, or when a
// group's prose does what the rendering does.
//
// Check I7 of the i18n plan — "the rendered skill carries every flag in the
// verb table" — needs no test any more: a signature is rendered from the
// flags a verb registers and there is nowhere else to write one. What can
// still go wrong is a verb with no Desc, which reaches the manual as a bare
// signature, and prose that lists verbs or flags by hand, which is how the
// second copy grew last time.
func CheckDescribed(t TB, c Command) {
	t.Helper()
	verbs := c.all()
	for _, name := range sortedVerbs(verbs) {
		described(t, c, verbs[name], name)
		for line := range strings.SplitSeq(verbs[name].Usage, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "- ") {
				t.Errorf("%s: a usage.md lists something: %q. The verbs are rendered under the prose; a list here is a second copy of them", c.Name, strings.TrimSpace(line))
			}
		}
	}
}

// described reports a verb, or a subcommand, that says nothing about itself.
func described(t TB, c Command, v Verb, path string) {
	if len(v.Subs) == 0 {
		if v.Desc == "" {
			t.Errorf("%s %s has no Desc; it reaches the manual as a signature with nothing said about it", c.Name, path)
		}
		return
	}
	for _, sub := range sortedVerbs(v.Subs) {
		described(t, c, v.Subs[sub], path+" "+sub)
	}
}
