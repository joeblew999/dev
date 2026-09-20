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

// Adding a cloud has to be adding an entry to clouds and nothing else. That
// was true of the verbs and was not true of Name and PutSecret, which still
// dispatched with `if target == "fly"` — the switch this registry exists to
// have replaced, surviving in the two functions nobody looked at because they
// are called from secrets rather than from a verb.
//
// The test is not "does it work"; it is "is every cloud complete". A cloud
// added with a gap here fails at whichever call site reaches the nil first,
// which is a panic somewhere unrelated.
func TestEveryCloudIsWholeSoAddingOneIsOneEdit(t *testing.T) {
	if len(clouds) == 0 {
		t.Fatal("no clouds, so the dispatch answers nothing")
	}
	for name, c := range clouds {
		for what, missing := range map[string]bool{
			"URL":       c.URL == nil,
			"Deployed":  c.Deployed == nil,
			"Deploy":    c.Deploy == nil,
			"Logs":      c.Logs == nil,
			"Delete":    c.Delete == nil,
			"Name":      c.Name == nil,
			"PutSecret": c.PutSecret == nil,
		} {
			if missing {
				t.Errorf("cloud %q has no %s, so whatever calls it panics", name, what)
			}
		}
		// Smoke is the one a target may decline, and declining is saying why:
		// a Fly app has no local runtime, and a reader needs that sentence
		// rather than a nil.
		if c.Smoke == nil && c.NoSmoke == "" {
			t.Errorf("cloud %q cannot smoke and does not say why", name)
		}
	}
}
