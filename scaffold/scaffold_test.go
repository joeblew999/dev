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
	old := latest
	latest = func(tool string) string {
		if strings.HasSuffix(tool, "jdx/hk") {
			return "9.9.9"
		}
		return ""
	}
	t.Cleanup(func() { latest = old })
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
	if err := Init(&out, dir, "", "0.2.0", "RWQtest"); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"mise.toml":                `"packslip:github.com/joeblew999/dev" = { version = "0.2.0", pubkey = "RWQtest" }`,
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
	mise, _ := os.ReadFile(filepath.Join(dir, "mise.toml"))
	for _, want := range []string{`"packslip:github.com/jdx/hk" = "9.9.9"`, `"packslip:github.com/jdx/fnox" = "1.35.2"`} {
		if !strings.Contains(string(mise), want) {
			t.Errorf("pins: mise.toml lacks %q (resolved ones move, unresolved keep the template's)", want)
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
	err := Init(&out, dir, "", "9.9.9", "")
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
	err := Init(&bytes.Buffer{}, t.TempDir(), "x", "", "")
	if err == nil || !strings.Contains(err.Error(), "--pin") {
		t.Errorf("a hand-built dev pinned itself: %v", err)
	}
	if err := Init(&bytes.Buffer{}, t.TempDir(), "Bad_Name", "1.0.0", ""); err == nil || !strings.Contains(err.Error(), "--name") {
		t.Errorf("a bad name was accepted: %v", err)
	}
}

// An existing repo with a module at the root gets the stack and nothing that
// would break its build: no go.work, no nested module, its command untouched.
func TestInitFitsAnExistingRootModule(t *testing.T) {
	old := latest
	latest = func(string) string { return "" }
	t.Cleanup(func() { latest = old })
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/lib\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "lib", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Init(&out, dir, "lib", "1.0.0", "k"); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"go.work", "cmd/lib/go.mod", "cmd/lib/main_test.go"} {
		if _, err := os.Stat(filepath.Join(dir, absent)); err == nil {
			t.Errorf("%s was written into a root-module repo", absent)
		}
	}
	main, _ := os.ReadFile(filepath.Join(dir, "cmd", "lib", "main.go"))
	if string(main) != "package main\n" {
		t.Error("the existing command was rewritten")
	}
	for _, present := range []string{"hk.pkl", "session.toml", "mise.toml", ".claude/hooks/skill-gate"} {
		if _, err := os.Stat(filepath.Join(dir, present)); err != nil {
			t.Errorf("%s not written", present)
		}
	}
}
