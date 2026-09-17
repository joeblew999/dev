// What a command directory holds, read from the directory itself. Every stage
// asks this rather than being told, so one verb works on a plain command, a
// Worker and a UI alike.
// Package stage builds, runs and checks one command directory from what it
// finds there: a Go main, a package.json (Vite), gsx sources, a wrangler.toml
// (a Worker, one wasm per environment), a fly.toml (a Fly app). mise names a
// stage per directory;
// this does the rest, so a new command is new lines in mise.toml, not new
// tooling.
package stage

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/joeblew999/dev/internal/cloudflare"
	"github.com/joeblew999/dev/internal/fly"
)

// Dir is what one command directory contains.
type Dir struct {
	Path     string            // as given, e.g. cmd/gui
	Name     string            // its basename: the binary is .bin/<Name>
	Root     string            // the repo root, where .bin/ lives
	NPM      bool              // package.json: npm ci (when stale) and npm run build
	GSX      bool              // .gsx sources: go tool gsx generate
	Wrangler bool              // wrangler.toml: a Worker, wasm built per environment
	Fly      bool              // fly.toml: a Fly app, deployed from a Dockerfile
	WasmOnly bool              // every Go file is js && wasm: no native binary
	CLI      bool              // imports github.com/joeblew999/dev/cli: a verb-table command with a manual Build keeps current
	Mains    map[string]string // wrangler environment → its main, "" for the top level
}

// Inspect reads a command directory.
func Inspect(path string) (Dir, error) {
	root, err := os.Getwd()
	if err != nil {
		return Dir{}, err
	}
	path = filepath.Clean(path)
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		return Dir{}, fmt.Errorf("%s is not a directory", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Dir{}, err
	}
	// A repo that is one command builds from "."; its name is the directory's.
	d := Dir{Path: path, Name: filepath.Base(abs), Root: root}
	d.NPM = exists(filepath.Join(path, "package.json"))
	gsx, _ := filepath.Glob(filepath.Join(path, "*.gsx"))
	d.GSX = len(gsx) > 0 || exists(filepath.Join(path, "gsx.toml"))
	if exists(filepath.Join(path, cloudflare.ConfigFile)) {
		d.Wrangler = true
		d.Mains, err = mains(filepath.Join(path, cloudflare.ConfigFile))
		if err != nil {
			return d, err
		}
	}
	d.Fly = exists(filepath.Join(path, fly.ConfigFile))
	d.WasmOnly = wasmOnly(path)
	if !d.WasmOnly {
		d.CLI = importsCLI(path)
	}
	return d, nil
}

// cliPath is the package a command imports to get the verb table, and with
// it `<name> skill`: a directory that imports it has a manual to keep.
const cliPath = "github.com/joeblew999/dev/cli"

// importsCLI reports whether the directory's package depends on cliPath. A
// directory go list cannot read is not one; go build says why right after.
func importsCLI(dir string) bool {
	cmd := exec.Command(GoBin, "list", "-deps", "-f", "{{.ImportPath}}", ".")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return slices.Contains(strings.Split(string(out), "\n"), cliPath)
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func mains(file string) (map[string]string, error) {
	var cfg struct {
		Main string `toml:"main"`
		Env  map[string]struct {
			Main string `toml:"main"`
		} `toml:"env"`
	}
	if _, err := toml.DecodeFile(file, &cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if cfg.Main == "" {
		return nil, fmt.Errorf("%s has no main", file)
	}
	m := map[string]string{"": cfg.Main}
	for name, e := range cfg.Env {
		m[name] = cfg.Main
		if e.Main != "" {
			m[name] = e.Main
		}
	}
	return m, nil
}

// wasmOnly reports whether the directory's Go files are all behind js && wasm,
// as a Worker entry point's are.
func wasmOnly(dir string) bool {
	cmd := exec.Command(GoBin, "list", "-f", "{{.Name}}", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return err != nil && strings.Contains(string(out), "build constraints exclude all Go files")
}
