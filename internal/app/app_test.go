package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetIsReadFromTheDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, d := range []string{"cf", "fl", "both", "none"} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"cf/wrangler.toml", "fl/fly.toml", "both/wrangler.toml", "both/fly.toml"} {
		if err := os.WriteFile(f, []byte("name = \"x\"\napp = \"x\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := Target("cf"); err != nil || got != "cloudflare" {
		t.Errorf("cf: %q, %v", got, err)
	}
	if got, err := Target("fl"); err != nil || got != "fly" {
		t.Errorf("fl: %q, %v", got, err)
	}
	if _, err := Target("both"); err == nil || !strings.Contains(err.Error(), "split it in two") {
		t.Errorf("both: %v", err)
	}
	if _, err := Target("none"); err == nil || !strings.Contains(err.Error(), "add one beside its main.go") {
		t.Errorf("none: %v", err)
	}
	if _, err := Target(filepath.Join("none", "missing")); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("missing: %v", err)
	}
}

func TestNameFollowsTheTarget(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("fl", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("fl/fly.toml", []byte("app = \"acme\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := Name("fl", ""); err != nil || got != "acme" {
		t.Errorf("Name = %q, %v", got, err)
	}
}
