package stage

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/joeblew999/dev/cli"
)

func TestInspectReadsWhatADirectoryHolds(t *testing.T) {
	t.Chdir(t.TempDir())
	os.MkdirAll("cmd/gui", 0o755)
	os.WriteFile("go.mod", []byte("module x\n\ngo 1.27\n"), 0o644)
	os.WriteFile("cmd/gui/main.go", []byte("package main\n\nfunc main() {}\n"), 0o644)
	os.WriteFile("cmd/gui/package.json", []byte("{}"), 0o644)
	os.WriteFile("cmd/gui/app.gsx", []byte(""), 0o644)
	os.WriteFile("cmd/gui/wrangler.toml", []byte("name = \"gui\"\nmain = \"build/go/worker.mjs\"\n[env.tinygo]\nmain = \"build/tinygo/worker.mjs\"\n[env.live]\nroutes = []\n"), 0o644)
	d, err := Inspect("cmd/gui")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "gui" || !d.NPM || !d.GSX || !d.Wrangler || d.WasmOnly {
		t.Fatalf("got %+v", d)
	}
	want := map[string]string{"": "build/go/worker.mjs", "tinygo": "build/tinygo/worker.mjs", "live": "build/go/worker.mjs"}
	for env, main := range want {
		if d.Mains[env] != main {
			t.Errorf("env %q: got %q, want %q", env, d.Mains[env], main)
		}
	}

	os.MkdirAll("cmd/worker", 0o755)
	os.WriteFile("cmd/worker/main.go", []byte("//go:build js && wasm\n\npackage main\n\nfunc main() {}\n"), 0o644)
	w, err := Inspect("cmd/worker")
	if err != nil {
		t.Fatal(err)
	}
	if !w.WasmOnly || w.NPM || w.GSX || w.Wrangler {
		t.Fatalf("worker: got %+v", w)
	}
	if _, err := Inspect("cmd/nothing"); err == nil {
		t.Fatal("a missing directory was accepted")
	}
}

func TestStale(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "lock"), filepath.Join(dir, "installed")
	os.WriteFile(in, nil, 0o644)
	if !stale(in, out) {
		t.Fatal("a missing output is not stale")
	}
	os.WriteFile(out, nil, 0o644)
	if stale(in, out) {
		t.Fatal("a fresh output is stale")
	}
	if stale(filepath.Join(dir, "no-input"), out) {
		t.Fatal("no input means nothing to do, not stale")
	}
}

// A directory whose module imports dev/cli is a verb-table command: Inspect
// says so, and Build then writes its manual. The module is built here with a
// replace to this checkout, so nothing is fetched.
func TestInspectSeesACLICommand(t *testing.T) {
	dev, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile(filepath.Join(dev, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	os.WriteFile("mise.toml", nil, 0o644)
	os.MkdirAll("cmd/plain", 0o755)
	os.WriteFile("cmd/plain/go.mod", []byte("module x/cmd/plain\n\ngo 1.27\n"), 0o644)
	os.WriteFile("cmd/plain/main.go", []byte("package main\n\nfunc main() {}\n"), 0o644)
	os.MkdirAll("cmd/tool", 0o755)
	os.WriteFile("cmd/tool/go.mod", []byte("module x/cmd/tool\n\ngo 1.27.1\n\nrequire github.com/joeblew999/dev v0.0.0\n\nreplace github.com/joeblew999/dev => "+dev+"\n"), 0o644)
	os.WriteFile("cmd/tool/go.sum", sum, 0o644)
	os.WriteFile("cmd/tool/main.go", []byte(`package main

import "github.com/joeblew999/dev/cli"

func main() { cli.Main(cli.Command{Name: "tool"}) }
`), 0o644)

	for path, want := range map[string]bool{"cmd/plain": false, "cmd/tool": true} {
		d, err := Inspect(path)
		if err != nil {
			t.Fatalf("Inspect(%s): %v", path, err)
		}
		if d.CLI != want {
			t.Errorf("Inspect(%s).CLI = %v, want %v", path, d.CLI, want)
		}
	}
	if err := Build(io.Discard, "cmd/tool", false, ""); err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, dir := range []string{cli.ShippedDir, cli.ClaudeDir, cli.AgentsDir} {
		p := filepath.Join(dir, "tool", cli.SkillFile)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("after Build, %s: %v", p, err)
		}
	}
}
