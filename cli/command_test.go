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
	stage := "tool build DIR    build it\ntool check DIR    check it\n"
	return Command{
		Name:    "tool",
		Version: "1.2.3",
		Head:    "---\nname: tool\n---\n\n# tool\n\n",
		Tail:    "## Rules\n\n- one\n",
		Verbs: map[string]Verb{
			"build": {Run: func(verb string, args []string, stdout, stderr io.Writer) error {
				_, err := stdout.Write([]byte("built\n"))
				return err
			}, Usage: stage},
			"check": {Run: func(verb string, args []string, stdout, stderr io.Writer) error {
				if len(args) == 0 {
					return Usagef("check: the directory comes first")
				}
				return errors.New("boom")
			}, Usage: stage},
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
		{nil, 2, "", "tool build DIR"},
		{[]string{"nope"}, 2, "", `unknown verb "nope"`},
		{[]string{"build"}, 0, "built\n", ""},
		{[]string{"check"}, 2, "", "error: check: the directory comes first\n\ntool build DIR"},
		{[]string{"check", "."}, 1, "", "error: boom\n"},
		{[]string{"version"}, 0, "1.2.3\n", ""},
		{[]string{"version", "x"}, 2, "", "error: tool version takes no arguments"},
	}
	for _, tc := range cases {
		var out, errb bytes.Buffer
		if code := c.run(tc.args, &out, &errb); code != tc.code {
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
	for _, want := range []string{"tool skill [--check]", "tool version", "tool build DIR"} {
		if strings.Count(idx, want) != 1 {
			t.Errorf("index has %q %d times, want once:\n%s", want, strings.Count(idx, want), idx)
		}
	}
}

func TestDefault(t *testing.T) {
	c := testCommand()
	c.Default = "build"
	for _, args := range [][]string{nil, {"-v"}} {
		var out, errb bytes.Buffer
		if code := c.run(args, &out, &errb); code != 0 || out.String() != "built\n" {
			t.Errorf("%v with Default: exit %d, stdout %q, stderr %q", args, code, out.String(), errb.String())
		}
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
	if code := c.run([]string{"skill", "--check"}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "is stale; regenerate it with: tool skill") {
		t.Fatalf("check before write: exit %d, stderr %q", code, errb.String())
	}
	if code := c.run([]string{"skill"}, &out, &errb); code != 0 {
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
	if code := c.run([]string{"skill", "--check"}, &out, &errb); code != 0 || strings.Count(out.String(), "is up to date") != 3 {
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
	if code := c.run([]string{"skill", "--check"}, &out, &errb); code != 1 || !strings.Contains(errb.String(), filepath.Join(ClaudeDir, "tool", SkillFile)+" is stale") {
		t.Errorf("check with a stale copy: exit %d, stderr %q", code, errb.String())
	}
	tb = &fakeTB{}
	if CheckSkill(tb, c); len(tb.errs) != 1 {
		t.Errorf("CheckSkill with a stale copy: %d errors, want 1", len(tb.errs))
	}
	if code := c.run([]string{"skill"}, &out, &errb); code != 0 {
		t.Fatalf("skill: exit %d", code)
	}
	if got, _ := os.ReadFile(local); string(got) != c.render() {
		t.Error("skill did not repair the stale copy")
	}
}
