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
	c.holdTo(t, want, shipped, claude, agents)
	dir, err := root(".")
	if err != nil {
		return
	}
	for name, body := range c.Skills {
		c.holdTo(t, withProvenance(body, c.Name), skillPaths(dir, name)...)
	}
}

// holdTo fails the test for every named file that is not what it should be.
// A manual's three copies and a shipped skill's three are the same question
// asked twice, and asking it twice is how two wordings of one message appear.
func (c Command) holdTo(t TB, want string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		have, err := os.ReadFile(p)
		if err != nil || string(have) != want {
			t.Errorf("%s is stale; regenerate it with: %s skill", rel(p), c.Name)
		}
	}
}

// The constructs CheckUsage rejects. Each is either something Flatten would
// pass through as visible noise (a fence, a table, a link) or something it
// would silently mangle (a numbered or nested list, whose shape it does not
// read). Keeping the list closed is what stops Flatten growing into a
// markdown engine: a new construct is a decision, not a patch.
//
// banned is a shape the terminal rendering cannot read, and the name it is
// called in the complaint. Both tables below declared this same struct.
type banned struct {
	name string
	re   *regexp.Regexp
}

// bannedLines are whole-line shapes, matched on the line as written. They
// cannot be matched after inline code is blanked, because a fence is itself
// backticks: blanking would eat it and the check would pass.
var bannedLines = []banned{
	{"a code fence", regexp.MustCompile("^\\s*```")},
	{"a table", regexp.MustCompile(`^\s*\|`)},
	{"a numbered list", regexp.MustCompile(`^\s*\d+\.\s`)},
	{"a nested list", regexp.MustCompile(`^\s+[-*+]\s`)},
	{"a blockquote", regexp.MustCompile(`^\s*>`)},
	{"a setext heading", regexp.MustCompile(`^\s*(=+|-{2,})\s*$`)},
}

// bannedSpans are inline shapes, matched only outside inline code, so that
// what a code span protects is never mistaken for markup.
var bannedSpans = []banned{
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
	for _, name := range SortedKeys(verbs) {
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
	for i, line := range Lines(md) {
		// Some patterns are read on the line as written and some with inline
		// code removed, because a `*` inside backticks is literal while a
		// heading is a heading wherever it is. The complaint is the same
		// either way, so it is written once.
		bare := inlineCode.ReplaceAllString(line, "")
		for _, scan := range []struct {
			banned []banned
			text   string
		}{{bannedLines, line}, {bannedSpans, bare}} {
			for _, b := range scan.banned {
				if b.re.MatchString(scan.text) {
					out = append(out, fmt.Sprintf("line %d uses %s, which the terminal rendering does not read: %q", i+1, b.name, line))
				}
			}
		}
	}
	return append(out, angleProblems(md)...)
}

// angleProblems is every <...> a markdown renderer would eat, as messages
// naming the line and its fix.
func angleProblems(md string) []string {
	var out []string
	for i, line := range Lines(md) {
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
	for _, name := range SortedKeys(verbs) {
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
	for _, sub := range SortedKeys(v.Subs) {
		described(t, c, v.Subs[sub], path+" "+sub)
	}
}

// CheckSurfaces fails the test when a verb reaches one of a command's
// surfaces and not the others.
//
// The terminal index, the skill a repo's agents read and the llms.txt a docs
// site serves are one render three ways: the terminal flattens the markdown
// so a shell can show it with no renderer, and the llms.txt turns each verb
// into a link to the manual. Nothing said so, and nothing would have noticed
// a change that let them part — a group filtered on one path and not the
// others, say. Then a verb would exist for a developer and not for an agent,
// or for neither and only for a model, and the only way to find out would be
// for someone to go looking.
//
// The third one is why this is a list rather than two comparisons: adding a
// surface is adding a line here, and every repo on the stack is held to it
// the next time it runs its tests.
//
// A command's main_test.go calls it, so `go test` — and so `dev check` —
// holds them together for every repo on the stack rather than for the one
// that happened to write the test.
func CheckSurfaces(t TB, c Command) {
	t.Helper()
	index, skill, llms := c.index(), c.render(), c.LLMs("").String()
	for name, v := range c.all() {
		sig := c.Name + " " + name
		if v.Args != "" {
			sig += " " + v.Args
		}
		for _, surface := range []struct{ what, text, who string }{
			{"the terminal index", index, "a developer"},
			{"the skill", skill, "an agent"},
			{"the llms.txt", llms, "a model reading the docs site"},
		} {
			if !strings.Contains(surface.text, sig) {
				t.Errorf("%q is not in %s, so %s cannot find it", sig, surface.what, surface.who)
				continue
			}
			// A signature with no sentence under it is a verb nobody knows
			// when to use, which is half of not being there at all.
			if v.Desc != "" && !strings.Contains(surface.text, v.Desc) {
				t.Errorf("%s: %s has the signature and not the description", name, surface.what)
			}
		}
	}
}

// CheckPinned holds a repo's pins to the tool registry, in both directions
// a pin can be wrong: the registry disagreeing with itself, and the registry
// disagreeing with the mise.toml that is actually read.
//
// Both facts — what a program is called to mise, and which version to take —
// live in cli.Needs, and a repo's mise.toml is a second copy of them. The
// copy is unavoidable: mise reads TOML and cannot call Go. What is avoidable
// is the two drifting in silence, and they did — seven checkers wrote their
// own pins by hand, in TOML, with nothing holding them to the file mise
// actually reads.
//
// Only the intersection, and deliberately: a repo pins what it will run, and
// one that never deploys to Fly should not be failed for having no flyctl.
// What this catches is a version bumped in one place and not the other, and
// a backend path mistyped in either.
func CheckPinned(t TB, misePath string) {
	t.Helper()
	config, err := os.ReadFile(misePath)
	if err != nil {
		t.Errorf("cannot read %s: %v", misePath, err)
		return
	}
	pinned := miseTools(string(config))
	for _, n := range Needs() {
		if n.Pin == "" {
			if n.Key != "" {
				t.Errorf("%s names a mise key and mise cannot install it", n.Bin)
			}
			continue
		}
		key, version, ok := strings.Cut(n.Pin, "@")
		if !ok || key == "" || version == "" {
			t.Errorf("%s's pin is not tool@version: %q", n.Bin, n.Pin)
			continue
		}
		// What mise calls a tool and what the binary is called are not always
		// the same, and getting it wrong sends somebody to install what they
		// already have: opentofu ships tofu, node ships npm. Read the way
		// mise's own listing is read, so the two cannot disagree.
		if got, want := n.MiseKey(), miseBinary(key); got != want {
			t.Errorf("%s asks mise about %q and its pin installs %q; a repo that pins it would still read as missing", n.Bin, got, want)
		}
		// And the [tools] line is derived from the same fact, so the two
		// cannot drift — which they did when both were written by hand.
		if line := n.Line(); !strings.Contains(line, version) {
			t.Errorf("%s's line %q does not name the version its pin does", n.Bin, line)
		}
		switch was, there := pinned[key]; {
		case !there:
		case was != version:
			t.Errorf("%s pins %s at %q and the registry says %q.\nOne of them is stale, and the registry is the one every other repo reads:\n  %s",
				misePath, key, was, version, n.Line())
		}
	}
}

// miseTools is the [tools] table: the name mise is given, and the version
// asked for. A small reader rather than a TOML library, because the section
// is flat and one key per line, and the dependency would be carried by every
// command on the stack for the sake of it.
func miseTools(config string) map[string]string {
	tools, inTools := map[string]string{}, false
	for line := range strings.Lines(config) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			// A table header, and only the flat [tools] one is pins: a
			// [tools.something] sub-table is settings for one of them.
			inTools = trimmed == "[tools]"
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !inTools || !ok || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// The comment this repo writes beside a pin is not part of the
		// version, and a key is quoted because of the : and / in a backend
		// path, not because mise calls it that.
		value, _, _ = strings.Cut(value, "#")
		tools[unquoted(key)] = version(value)
	}
	return tools
}

// unquoted is one TOML string: trimmed, and without the quotes it was
// written with.
func unquoted(s string) string { return strings.Trim(strings.TrimSpace(s), `"`) }

// version is what a [tools] value asks for, whether it is written as a string
// or as a table.
//
// mise lets a pin carry settings — wrangler needs
// allow_builds = ["esbuild", "sharp", "workerd"] before npm will install it —
// and then the line is an inline table rather than a version. Reading
// everything after the first = as the version made that whole brace-blob the
// version, so a repo that legitimately pins wrangler with its build allowance
// was told its pin disagreed with a registry that says the same thing. Any
// repo on this stack using allow_builds would have hit it.
func version(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "{") {
		return unquoted(value)
	}
	// One field out of the table, by name: the others are how to install it,
	// not which one to install.
	_, rest, ok := strings.Cut(value, "version")
	if !ok {
		return ""
	}
	if _, rest, ok = strings.Cut(rest, `"`); !ok {
		return ""
	}
	got, _, _ := strings.Cut(rest, `"`)
	return got
}
