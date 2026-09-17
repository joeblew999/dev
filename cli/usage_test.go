package cli

import (
	"flag"
	"io"
	"strings"
	"testing"
)

// The angle-bracket rule is why CheckUsage exists, so it is tested first.
// `<app>.fly.dev` and `NAME<TAB>OWNER` are real text from this tool's usage:
// unfenced, a markdown renderer swallows the tag and the reader loses the
// placeholder without any error anywhere. This test is the error.
func TestCheckUsageCatchesBareAngleBrackets(t *testing.T) {
	for _, line := range []string{
		"- `dev url DIR`\n  a Fly app's is <app>.fly.dev",
		"- `dev secrets push DIR`\n  reads NAME<TAB>OWNER lines on stdin",
		"the namespaces are titled <worker>-<binding>",
	} {
		problems := usageProblems(line)
		if len(problems) == 0 {
			t.Errorf("no problem reported for bare angle brackets in %q", line)
			continue
		}
		if !strings.Contains(problems[0], "HTML tag") {
			t.Errorf("problem should say why it matters, got %q", problems[0])
		}
	}
}

// In inline code the same text escapes correctly, so it must pass.
func TestCheckUsageAllowsAngleBracketsInCode(t *testing.T) {
	for _, line := range []string{
		"- `dev url DIR`\n  a Fly app's is `<app>.fly.dev`",
		"- `dev secrets push DIR`\n  reads `NAME<TAB>OWNER` lines on stdin",
		"the namespaces are titled `<worker>-<binding>`",
	} {
		if problems := usageProblems(line); len(problems) > 0 {
			t.Errorf("%q is correct markdown but was rejected: %v", line, problems)
		}
	}
}

// Every construct Flatten cannot read must be refused, or the manual and the
// terminal say different things.
func TestCheckUsageRejectsWhatFlattenCannotRead(t *testing.T) {
	for name, md := range map[string]string{
		"a code fence":    "```\ndev build DIR\n```",
		"a table":         "| verb | does |\n|---|---|",
		"a numbered list": "1. `dev build DIR`",
		"a nested list":   "- `dev build DIR`\n  - a sub-point",
		"a blockquote":    "> note this",
		"a link":          "- `dev build DIR`\n  see [the docs](http://x)",
		"emphasis":        "- `dev build DIR`\n  this is *important*",
		"bold":            "- `dev build DIR`\n  this is **important**",
	} {
		if problems := usageProblems(md); len(problems) == 0 {
			t.Errorf("%s was accepted but Flatten cannot read it: %q", name, md)
		}
	}
}

// Globs are the reason emphasis is banned rather than unwound: `**/*.go` and
// `secrets:*` are text this tool really prints, and a flattener that stripped
// * would turn the first into "/*.go".
func TestFlattenKeepsGlobsIntact(t *testing.T) {
	got := Flatten("- `dev check DIR`\n  vets `**/*.go`, and `secrets:*` names the rest")
	want := "dev check DIR\n    vets **/*.go, and secrets:* names the rest\n"
	if got != want {
		t.Errorf("Flatten mangled a glob:\n got %q\nwant %q", got, want)
	}
}

// The shape the whole design rests on: a list item flattens to a signature
// with its description indented under it, which is what the usage text looked
// like before it was markdown.
func TestFlattenIndentsDescriptionsUnderSignatures(t *testing.T) {
	md := "### Deploying\n\n" +
		"- `dev url DIR [--env NAME]`\n" +
		"  print the URL to talk to: the deployed app in DIR,\n" +
		"  else `--local` (default empty)\n" +
		"- `dev logs DIR`\n" +
		"  stream the deployed app's logs\n\n" +
		"Run from the repo root.\n"
	want := "Deploying\n\n" +
		"dev url DIR [--env NAME]\n" +
		"    print the URL to talk to: the deployed app in DIR,\n" +
		"    else --local (default empty)\n" +
		"dev logs DIR\n" +
		"    stream the deployed app's logs\n\n" +
		"Run from the repo root.\n"
	if got := Flatten(md); got != want {
		t.Errorf("Flatten:\n got %q\nwant %q", got, want)
	}
}

// The rendered manual is committed and hk's trailing_whitespace reads it, so
// Flatten must never emit a line ending in a space — including a continuation
// that flattened to nothing but its indent.
func TestFlattenNeverEmitsTrailingWhitespace(t *testing.T) {
	md := "### Heading   \n\n- `dev build DIR`   \n  a description   \n  `` \n\nA paragraph   \n"
	for i, line := range strings.Split(Flatten(md), "\n") {
		if line != strings.TrimRight(line, " \t") {
			t.Errorf("line %d ends in whitespace: %q", i+1, line)
		}
	}
}

// An empty Order must render exactly as it did before Order existed: every
// consumer repo's command has none, so this is the compatibility guarantee.
func TestEmptyOrderRendersAlphabetically(t *testing.T) {
	c := Command{Name: "x", Verbs: map[string]Verb{
		"zebra": {Usage: "z\n"}, "alpha": {Usage: "a\n"}, "middle": {Usage: "m\n"},
	}}
	if got, want := c.manualOrder(c.all()), []string{"alpha", "middle", "skill", "skills", "version", "zebra"}; !equal(got, want) {
		t.Errorf("empty Order: got %v, want %v", got, want)
	}
}

// Order puts named verbs first and leaves the rest in name order behind them.
func TestOrderLeadsAndTheRestFollow(t *testing.T) {
	c := Command{Name: "x", Order: []string{"zebra", "alpha"}, Verbs: map[string]Verb{
		"zebra": {Usage: "z\n"}, "alpha": {Usage: "a\n"}, "middle": {Usage: "m\n"},
	}}
	if got, want := c.manualOrder(c.all()), []string{"zebra", "alpha", "middle", "skill", "skills", "version"}; !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A stale name in Order — a verb since renamed — must not break the build;
// the manual just falls back to name order for it.
func TestOrderIgnoresUnknownVerbs(t *testing.T) {
	c := Command{Name: "x", Order: []string{"gone", "alpha"}, Verbs: map[string]Verb{
		"alpha": {Usage: "a\n"}, "middle": {Usage: "m\n"},
	}}
	if got, want := c.manualOrder(c.all()), []string{"alpha", "middle", "skill", "skills", "version"}; !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The bug this check exists for: tail.md shipped `NAME<TAB>OWNER` unfenced,
// so every rendered copy of the manual showed "NAMEOWNER" and lost the fact
// that the lines are tab-separated. Nothing caught it.
func TestCheckUsageCatchesBareAngleBracketsInProse(t *testing.T) {
	c := Command{
		Name:  "x",
		Verbs: map[string]Verb{"go": {Usage: "- `x go`\n  fine\n"}},
		Head:  "---\nname: x\n---\n\n# x\n\nprose\n",
		Tail:  "- A task printing NAME<TAB>OWNER lines\n",
	}
	var f fakeTB
	CheckUsage(&f, c)
	if len(f.errs) != 1 {
		t.Fatalf("want exactly the tail problem, got %v", f.errs)
	}
	if !strings.Contains(f.errs[0], "x tail:") || !strings.Contains(f.errs[0], "HTML tag") {
		t.Errorf("message should name the prose and why: %q", f.errs[0])
	}
}

// Frontmatter is YAML the renderer never sees, and its --- would otherwise
// read as a setext heading.
func TestFrontmatterIsNotChecked(t *testing.T) {
	c := Command{
		Name:  "x",
		Verbs: map[string]Verb{"go": {Usage: "- `x go`\n  fine\n"}},
		Head:  "---\nname: x\ndescription: a <thing> in metadata\n---\n\n# x\n",
		Tail:  "",
	}
	var f fakeTB
	CheckUsage(&f, c)
	if len(f.errs) != 0 {
		t.Errorf("frontmatter should not be checked, got %v", f.errs)
	}
}

// Head and Tail only ever reach the manual, never a terminal, so markdown
// Flatten cannot read is still correct there.
func TestProseMayUseMarkdownFlattenCannotRead(t *testing.T) {
	c := Command{
		Name:  "x",
		Verbs: map[string]Verb{"go": {Usage: "- `x go`\n  fine\n"}},
		Head:  "# x\n\n| a | b |\n|---|---|\n\nsee [docs](http://x), *emphasised*\n",
		Tail:  "> a note\n\n1. a numbered list\n",
	}
	var f fakeTB
	CheckUsage(&f, c)
	if len(f.errs) != 0 {
		t.Errorf("prose is markdown-only and may use all of it, got %v", f.errs)
	}
}

// Flatten must leave legacy text alone: the terminal showed it correctly
// before and has to keep doing so while a repo is unported.
func TestFlattenLeavesLegacyTextUnchanged(t *testing.T) {
	legacy := "hello serve [--addr HOST:PORT]    answer /health\n" +
		"hello deploy DIR\n    deploy what DIR holds, and say what was created\n"
	if got := Flatten(legacy); got != legacy {
		t.Errorf("Flatten changed legacy text:\n got %q\nwant %q", got, legacy)
	}
}

// Asking what a verb takes is not an error. It used to exit 2 with
// "flag: help requested", which is how the flags' own descriptions — the only
// place that says what --path or --env mean — stayed invisible.
func TestHelpIsNotAnError(t *testing.T) {
	c := testHelpCommand()
	var out, errOut strings.Builder
	if code := c.run([]string{"go", "--help"}, &out, &errOut); code != 0 {
		t.Errorf("--help exited %d, want 0", code)
	}
	if !strings.Contains(out.String(), "what it is for") {
		t.Errorf("--help should print the verb's own usage, got %q", out.String())
	}
}

func testHelpCommand() Command {
	return Command{Name: "x", Verbs: map[string]Verb{
		"go": {
			Desc: "what it is for",
			Run:  func(string, []string, io.Writer, io.Writer) error { return ErrHelp },
		},
	}}
}

// `<cmd> --help` is a question and gets an answer: the index, on stdout,
// exit 0. `<cmd>` with no verb is a mistake and gets a correction: the same
// text on stderr, exit 2. The difference is whether something was meant to
// run, and a task that only wants to show the manual must not look failed.
func TestTopLevelHelpAnswersRatherThanCorrects(t *testing.T) {
	c := testHelpCommand()
	for _, tc := range []struct {
		args   []string
		code   int
		stdout bool
	}{
		{[]string{"--help"}, 0, true},
		{[]string{"-h"}, 0, true},
		{nil, 2, false},
	} {
		var out, errOut strings.Builder
		got := c.run(tc.args, &out, &errOut)
		if got != tc.code {
			t.Errorf("run(%v) exited %d, want %d", tc.args, got, tc.code)
		}
		if tc.stdout && !strings.Contains(out.String(), "x go") {
			t.Errorf("run(%v) should answer on stdout, got %q", tc.args, out.String())
		}
		if !tc.stdout && errOut.String() == "" {
			t.Errorf("run(%v) should correct on stderr", tc.args)
		}
	}
}

// Everything after a bare -- belongs to the program being run, so its --help
// is not ours to answer.
func TestHelpRequestedStopsAtTheDoubleDash(t *testing.T) {
	if !HelpRequested([]string{"DIR", "--help"}) {
		t.Error("--help before -- is ours")
	}
	if HelpRequested([]string{"DIR", "--", "--help"}) {
		t.Error("--help after -- belongs to the program being run")
	}
}

// A verb's line is rendered from its own Desc, beside the signature rendered
// from its Args and Flags. Nothing is read out of a usage.md to find it, so
// the two cannot disagree about which verb they describe.
func TestVerbsRendersSignatureAndDescription(t *testing.T) {
	c := Command{Name: "x", Verbs: map[string]Verb{
		"go": {Args: "DIR", Desc: "do the thing", Flags: func(fs *flag.FlagSet) {
			fs.String("mode", "", "the `MODE` to use")
		}},
	}}
	got := Verbs("x", c.all(), []string{"go"})
	want := "- `x go DIR [--mode MODE]`\n  do the thing\n"
	if got != want {
		t.Errorf("Verbs:\n got %q\nwant %q", got, want)
	}
}

// A group's prose explains why its verbs exist; it must not list them, or the
// list becomes a second copy of what is rendered under it.
func TestProseMayNotListVerbs(t *testing.T) {
	c := Command{Name: "x", Verbs: map[string]Verb{
		"go": {Desc: "do it", Usage: "### Doing\n\n- go\n  do it\n"},
	}}
	var f fakeTB
	CheckDescribed(&f, c)
	if len(f.errs) == 0 {
		t.Error("a usage.md listing a verb should fail")
	}
}

// A verb with nothing said about it reaches the manual as a bare signature.
func TestVerbMustSayWhatItIsFor(t *testing.T) {
	c := Command{Name: "x", Verbs: map[string]Verb{"go": {Args: "DIR"}}}
	var f fakeTB
	CheckDescribed(&f, c)
	if len(f.errs) == 0 {
		t.Error("a verb with no Desc should fail")
	}
}
