package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The llms.txt is the third rendering of one declaration, so what it says has
// to come from the declaration and nowhere else: the summary from the
// frontmatter the manual already carries, the sections from the groups' own
// headings, the signatures and the lines under them from the verbs.
//
// CheckSurfaces is what holds every verb to it — this says the document is
// the shape llmstxt.org asks for, which CheckSurfaces cannot see.
func TestLLMsDescribesTheCommandItself(t *testing.T) {
	c := testCommand()
	c.Skill = "---\nname: tool\ndescription: what tool is for: a colon and all.\n---\n\n# tool\n\n<!-- verbs -->\n"
	c.Skills = map[string]string{"lib": "---\nname: lib\ndescription: the library.\n---\n\n# lib\n"}

	got := c.LLMs("https://example.com/").String()
	for _, want := range []string{
		"# tool\n",
		// The value keeps the colons inside it: a description is a sentence,
		// not a key and a value.
		"> what tool is for: a colon and all.\n",
		// The group's own heading names the section, rather than the llms.txt
		// inventing a second name for it.
		"\n## Stages\n\n",
		"- [tool build](https://example.com/skills/tool/SKILL.md): build it\n",
		"- [tool check DIR](https://example.com/skills/tool/SKILL.md): check it\n",
		"\n## Manuals\n\n",
		"- [lib](https://example.com/skills/lib/SKILL.md): the library.\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the llms.txt does not carry %q:\n%s", want, got)
		}
	}

	// Nothing published yet: the links are the paths skill writes, which is
	// what a reader has to go on.
	if local := c.LLMs("").String(); !strings.Contains(local, "](skills/tool/SKILL.md)") {
		t.Errorf("with no origin the links should be the repo's own paths:\n%s", local)
	}

	// A verb declared once reaches it, with nothing else edited.
	c.Verbs["ship"] = Verb{Run: func(Call) error { return nil }, Args: "DIR",
		Desc: "ship it", Usage: "### Shipping\n\nWhat it says.\n"}
	if after := c.LLMs("").String(); !strings.Contains(after, "- [tool ship DIR](skills/tool/SKILL.md): ship it") ||
		!strings.Contains(after, "## Shipping") {
		t.Errorf("a new verb did not reach the llms.txt:\n%s", after)
	}
}

// A command with no prose to draw on still writes a usable file: llmstxt.org
// wants a title and links, and a missing summary is a warning in every
// validator rather than a reason to write nothing.
func TestLLMsWithoutFrontmatterStillNamesTheCommand(t *testing.T) {
	c := Command{Name: "bare", Verbs: map[string]Verb{
		"go": {Run: func(Call) error { return nil }, Desc: "do it"},
	}}
	got := c.LLMs("").String()
	if strings.Contains(got, "\n>") {
		t.Errorf("a command with no description should have no summary line:\n%s", got)
	}
	// The group has no heading of its own, so the section is named after the
	// first verb in it rather than left blank.
	if !strings.HasPrefix(got, "# bare\n\n## go\n\n") {
		t.Errorf("want the command's name then a section named for its verb:\n%s", got)
	}
	if n := c.LLMs("").Links(); n != len(c.all())+1 {
		t.Errorf("Links() = %d; want one per verb plus the manual", n)
	}
}

// The verb writes where the convention is read from — the root of the site —
// and prints the document instead when no directory is named, so a build can
// pipe it without leaving a file behind.
func TestLLMsVerbWritesTheFileOrPrintsIt(t *testing.T) {
	c := testCommand()
	var out, errb bytes.Buffer
	if code := c.Run([]string{"llms"}, &out, &errb); code != 0 || !strings.HasPrefix(out.String(), "# tool\n") {
		t.Fatalf("llms: exit %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}

	site := t.TempDir()
	out.Reset()
	errb.Reset()
	if code := c.Run([]string{"llms", site, "--origin", "https://example.com"}, &out, &errb); code != 0 {
		t.Fatalf("llms DIR: exit %d, stderr %q", code, errb.String())
	}
	if out.String() != "" {
		t.Errorf("stdout carried %q; with a directory the document is the file, not the output", out.String())
	}
	written, err := os.ReadFile(filepath.Join(site, LLMsFile))
	if err != nil || string(written) != c.LLMs("https://example.com").String() {
		t.Errorf("%s: %v, or not what LLMs renders", LLMsFile, err)
	}
}
