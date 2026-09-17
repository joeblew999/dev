package scaffold

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWritesTheStackAndRefusesToOverwrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "widget")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://github.com/acme/widget.git"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	var out bytes.Buffer
	if err := Init(&out, dir, "", "0.2.0"); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"mise.toml":                `"packslip:github.com/joeblew999/dev" = "0.2.0"`,
		"cmd/widget/go.mod":        "module github.com/acme/widget/cmd/widget",
		"cmd/widget/main.go":       "hello from widget",
		"go.work":                  "./cmd/widget",
		"AGENTS.md":                "- Repo: acme/widget",
		".claude/hooks/skill-gate": "PreToolUse",
	} {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatalf("%s not written: %v", path, err)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s lacks %q", path, want)
		}
		if strings.Contains(string(data), "__") {
			t.Errorf("%s has an unfilled placeholder", path)
		}
	}
	if st, _ := os.Stat(filepath.Join(dir, ".claude/hooks/skill-gate")); st.Mode()&0o111 == 0 {
		t.Error("the hook is not executable")
	}
	if _, err := os.Stat(filepath.Join(dir, "mise.toml.tmpl")); err == nil {
		t.Error("a template suffix leaked into the repo")
	}

	// A second run touches nothing and says so.
	before, _ := os.ReadFile(filepath.Join(dir, "mise.toml"))
	out.Reset()
	err := Init(&out, dir, "", "9.9.9")
	if err == nil || !strings.Contains(err.Error(), "every file exists") {
		t.Errorf("second init = %v; want a refusal", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "mise.toml"))
	if !bytes.Equal(before, after) {
		t.Error("second init rewrote mise.toml")
	}
	if !strings.Contains(out.String(), "kept  "+filepath.Join(dir, "mise.toml")) {
		t.Errorf("second init did not name the kept file:\n%s", out.String())
	}
}

func TestInitNeedsAReleaseToPin(t *testing.T) {
	err := Init(&bytes.Buffer{}, t.TempDir(), "x", "")
	if err == nil || !strings.Contains(err.Error(), "--pin") {
		t.Errorf("a hand-built dev pinned itself: %v", err)
	}
	if err := Init(&bytes.Buffer{}, t.TempDir(), "Bad_Name", "1.0.0"); err == nil || !strings.Contains(err.Error(), "--name") {
		t.Errorf("a bad name was accepted: %v", err)
	}
}
