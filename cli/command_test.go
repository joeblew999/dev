package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCommand() Command {
	stage := "### Stages\n\nWhat every command goes through.\n"
	return Command{
		Name:    "tool",
		Version: "1.2.3",
		Skill:   "---\nname: tool\n---\n\n# tool\n\n\n<!-- verbs -->\n## Rules\n\n- one\n",
		Verbs: map[string]Verb{
			// build takes nothing, so the default verb can run with no
			// arguments at all; check takes a directory, which cli parses.
			"build": {Run: func(c Call) error {
				_, err := c.Stdout.Write([]byte("built\n"))
				return err
			}, Desc: "build it", Usage: stage},
			"check": {Run: func(c Call) error {
				return errors.New("boom")
			}, Args: "DIR", Desc: "check it", Usage: stage},
		},
	}
}

func TestRun(t *testing.T) {
	c := testCommand()
	cases := []struct {
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{nil, 2, "", "tool build"},
		{[]string{"nope"}, 2, "", `unknown verb "nope"`},
		{[]string{"build"}, 0, "built\n", ""},
		{[]string{"check"}, 2, "", "error: tool check: the directory comes first"},
		{[]string{"check", "."}, 1, "", "error: boom\n"},
		{[]string{"version"}, 0, "1.2.3\n", ""},
		{[]string{"version", "x"}, 2, "", "error: tool version: takes no arguments"},
	}
	for _, tc := range cases {
		var out, errb bytes.Buffer
		if code := c.Run(tc.args, &out, &errb); code != tc.code {
			t.Errorf("%v: exit %d, want %d (stderr %q)", tc.args, code, tc.code, errb.String())
		}
		if out.String() != tc.stdout {
			t.Errorf("%v: stdout %q, want %q", tc.args, out.String(), tc.stdout)
		}
		if !strings.Contains(errb.String(), tc.stderr) {
			t.Errorf("%v: stderr %q, want it to contain %q", tc.args, errb.String(), tc.stderr)
		}
	}
	// The index lists the command's own verbs beside the table's, each usage once.
	idx := c.index()
	for _, want := range []string{"tool skill [--check]", "tool version", "tool check DIR"} {
		if strings.Count(idx, want) != 1 {
			t.Errorf("index has %q %d times, want once:\n%s", want, strings.Count(idx, want), idx)
		}
	}
}

func TestDefault(t *testing.T) {
	c := testCommand()
	c.Default = "build"

	// No verb runs the default one.
	var out, errb bytes.Buffer
	if code := c.Run(nil, &out, &errb); code != 0 || out.String() != "built\n" {
		t.Errorf("no verb: exit %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}

	// So does a flag with no verb — and the default verb then judges the
	// flag, which is how an unknown one is caught rather than ignored. It
	// used to be ignored, because a verb that never parsed its arguments
	// could not tell -v from nothing.
	out.Reset()
	errb.Reset()
	if code := c.Run([]string{"-v"}, &out, &errb); code != 2 ||
		!strings.Contains(errb.String(), "not defined: -v") {
		t.Errorf("unknown flag: exit %d, stderr %q", code, errb.String())
	}
}

type fakeTB struct{ errs []string }

func (f *fakeTB) Helper() {}

// Errorf records the message, not the format string: a test asserting on
// what a check reports needs the file and the reason, which live in the
// arguments.
func (f *fakeTB) Errorf(format string, a ...any) { f.errs = append(f.errs, fmt.Sprintf(format, a...)) }

func TestSkillRoundTrip(t *testing.T) {
	c := testCommand()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "mise.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := filepath.Join(repo, "cmd", "tool")
	if err := os.MkdirAll(cmd, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cmd) // where go test would run: the manual still lands at the root

	var out, errb bytes.Buffer
	if code := c.Run([]string{"skill", "--check"}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "is stale; regenerate it with: tool skill") {
		t.Fatalf("check before write: exit %d, stderr %q", code, errb.String())
	}
	if code := c.Run([]string{"skill"}, &out, &errb); code != 0 {
		t.Fatalf("skill: exit %d, stderr %q", code, errb.String())
	}
	for _, dir := range []string{ShippedDir, ClaudeDir, AgentsDir} {
		p := filepath.Join(dir, "tool", SkillFile)
		got, err := os.ReadFile(filepath.Join(repo, p))
		if err != nil || string(got) != c.render() {
			t.Errorf("%s: %v, or not the rendered manual", p, err)
		}
	}
	out.Reset()
	if code := c.Run([]string{"skill", "--check"}, &out, &errb); code != 0 || strings.Count(out.String(), "is up to date") != 3 {
		t.Errorf("check after write: exit %d, stdout %q", code, out.String())
	}
	tb := &fakeTB{}
	if CheckSkill(tb, c); len(tb.errs) != 0 {
		t.Errorf("CheckSkill after write: %v", tb.errs)
	}

	// One copy edited by hand: check names it, CheckSkill fails, skill repairs it.
	local := filepath.Join(repo, ClaudeDir, "tool", SkillFile)
	if err := os.WriteFile(local, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	errb.Reset()
	if code := c.Run([]string{"skill", "--check"}, &out, &errb); code != 1 || !strings.Contains(errb.String(), filepath.Join(ClaudeDir, "tool", SkillFile)+" is stale") {
		t.Errorf("check with a stale copy: exit %d, stderr %q", code, errb.String())
	}
	tb = &fakeTB{}
	if CheckSkill(tb, c); len(tb.errs) != 1 {
		t.Errorf("CheckSkill with a stale copy: %d errors, want 1", len(tb.errs))
	}
	if code := c.Run([]string{"skill"}, &out, &errb); code != 0 {
		t.Fatalf("skill: exit %d", code)
	}
	if got, _ := os.ReadFile(local); string(got) != c.render() {
		t.Error("skill did not repair the stale copy")
	}
}

// The one instruction a reader follows before they have the tool is written
// by the tool, so it cannot name a key that no longer signs — which is how
// this repo's README came to name no key at all and install nothing.
func TestReadmeCarriesThePin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mise.toml"), []byte("[tools]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(dir, "README.md")
	const before = "# tool\n\n## Get it\n\n" + PinMarker + "\n" + PinMarker + "\n\nrest\n"
	if err := os.WriteFile(readme, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	c := Command{Name: "tool", Version: "1.2.3",
		Pin: "github.com/owner/tool", PubKey: "RWQtheKey"}

	var out bytes.Buffer
	if err := c.readme(&out, dir, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"packslip:github.com/owner/tool" = { version = "<version>", pubkey = "RWQtheKey" }`,
		"tool version --pin", "# tool", "rest",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("the written README does not carry %q:\n%s", want, got)
		}
	}

	// Written twice is written once: the check must pass straight after.
	if err := c.readme(io.Discard, dir, true); err != nil {
		t.Errorf("a freshly written README failed its own check: %v", err)
	}

	// A key that no longer signs is caught rather than left to rot.
	rotted := strings.ReplaceAll(string(got), "RWQtheKey", "RWQsomeOldKey")
	if err := os.WriteFile(readme, []byte(rotted), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.readme(io.Discard, dir, true); err == nil {
		t.Error("a stale key passed the check")
	}

	// A README with no marker is not this command's business.
	plain := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(plain, []byte("# someone else's\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.readme(io.Discard, filepath.Dir(plain), true); err != nil {
		t.Errorf("a README without the marker was touched: %v", err)
	}
}

// A command that does not say how it is pinned says so, rather than printing
// half a line someone would paste.
func TestVersionPinNeedsBoth(t *testing.T) {
	var out, errb bytes.Buffer
	plain := Command{Name: "tool", Version: "1.0.0", Verbs: map[string]Verb{}}
	if code := plain.Run([]string{"version", "--pin"}, &out, &errb); code != 1 {
		t.Errorf("exit %d; want 1 when there is no pin to print", code)
	}
	out.Reset()
	pinned := Command{Name: "tool", Version: "v1.0.0", Verbs: map[string]Verb{},
		Pin: "github.com/owner/tool", PubKey: "RWQkey"}
	if code := pinned.Run([]string{"version", "--pin"}, &out, &errb); code != 0 {
		t.Errorf("exit %d; want 0", code)
	}
	// The leading v is the tag's, not the version a pin carries.
	if !strings.Contains(out.String(), `version = "1.0.0"`) {
		t.Errorf("printed %q; want the version without its v", out.String())
	}
}

// The unification of the two surfaces is held by cli.CheckSurfaces, which
// every command on the stack calls from its own main_test.go rather than only
// this package testing its own synthetic command.
func TestBothSurfacesShowTheSameVerbs(t *testing.T) { CheckSurfaces(t, testCommand()) }
