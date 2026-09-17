// Package stage builds, runs and checks one command directory from what it
// finds there: a Go main, a package.json (Vite), gsx sources, a wrangler.toml
// (a Worker, one wasm per environment), a fly.toml (a Fly app). mise names a
// stage per directory;
// this does the rest, so a new command is new lines in mise.toml, not new
// tooling.
package stage

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/cloudflare"
	"github.com/joeblew999/dev/internal/fly"
	"github.com/joeblew999/dev/internal/gitignore"
)

// BinDir is where every command's binary is built and run from, under a dot
// so a repo root lists what a developer wrote and not what a tool made.
// mise.toml's outputs and .gitignore name the same path.
const BinDir = ".bin"

// The binaries stage drives: go builds and vets, npm builds the UI, node
// runs the browser probe, tinygo builds the Worker's wasm where it applies,
// gsx generates from .gsx sources, wrangler runs the Worker locally, fnox
// supplies secrets at run time.
const (
	GoBin       = "go"
	NpmBin      = "npm"
	NodeBin     = "node"
	TinyGoBin   = "tinygo"
	GsxTool     = "gsx"
	WranglerBin = "wrangler"
	FnoxBin     = "fnox"
)

// Env keys stage sets or reads.
const (
	GoOSEnv   = "GOOS"
	GoArchEnv = "GOARCH"
	GoPortEnv = "GO_PORT"
	ChromeEnv = "CHROME"
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
	for _, line := range strings.Split(string(out), "\n") {
		if line == cliPath {
			return true
		}
	}
	return false
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

// run runs a tool in dir. Progress and the tool's own output go to stderr:
// stdout is for data, and a task that depends on a build may be piped.
func run(out io.Writer, dir string, env []string, name string, args ...string) error {
	fmt.Fprintf(os.Stderr, "$ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s (in %s): %w", name, strings.Join(args, " "), dir, err)
	}
	return nil
}

// Build builds the directory's binary: npm and gsx first where they apply,
// then go build to .bin/<dir>. With asWorker it builds the Worker's wasm for
// env instead: the environment's main names the output directory, and one
// under build/tinygo is built with TinyGo, any other with Go.
func Build(out io.Writer, path string, asWorker bool, env string) error {
	d, err := Inspect(path)
	if err != nil {
		return err
	}
	if asWorker {
		return buildWorker(out, d, env)
	}
	if d.WasmOnly {
		return fmt.Errorf("%s builds only a Worker; build it with --worker", d.Path)
	}
	if d.NPM {
		if stale(filepath.Join(d.Path, "package-lock.json"), filepath.Join(d.Path, "node_modules", ".package-lock.json")) {
			if err := run(out, d.Path, nil, NpmBin, "ci"); err != nil {
				return err
			}
		}
		if err := run(out, d.Path, nil, NpmBin, "run", "build"); err != nil {
			return err
		}
	}
	if d.GSX {
		if err := run(out, d.Path, nil, GoBin, "tool", "gsx", "generate", "-q"); err != nil {
			return err
		}
	}
	if err := gitignore.Ensure(d.Root, BinDir); err != nil {
		return err
	}
	bin := filepath.Join(d.Root, BinDir, d.Name)
	if err := run(out, d.Path, nil, GoBin, "build", "-o", bin, "."); err != nil {
		return err
	}
	if d.CLI {
		// A verb-table command writes its own manual. Building it rewrites
		// every copy, so a changed verb reaches this repo's agents with no
		// step taken; go test holds them when nothing built.
		return run(out, d.Root, nil, bin, "skill")
	}
	return nil
}

func buildWorker(out io.Writer, d Dir, env string) error {
	if !d.Wrangler {
		return fmt.Errorf("%s has no wrangler.toml; it is not a Worker", d.Path)
	}
	if d.GSX {
		if err := run(out, d.Path, nil, GoBin, "tool", "gsx", "generate", "-q"); err != nil {
			return err
		}
	}
	var err error
	{
		main, ok := d.Mains[env]
		if !ok {
			return fmt.Errorf("%s/wrangler.toml has no environment %q", d.Path, env)
		}
		outDir := filepath.Dir(main)
		mode, goEnv := "go", []string{GoOSEnv + "=js", GoArchEnv + "=wasm"}
		if filepath.Base(outDir) == "tinygo" {
			mode = "tinygo"
		}
		if err := run(out, d.Path, nil, GoBin, "run", "github.com/syumai/workers-go/cmd/workers-assets-gen", "-mode="+mode, "-o", outDir); err != nil {
			return err
		}
		wasm := filepath.Join(outDir, "app.wasm")
		if mode == "tinygo" {
			err = run(out, d.Path, nil, TinyGoBin, "build", "-o", wasm, "-target", "wasm", "-no-debug", ".")
		} else {
			err = run(out, d.Path, goEnv, GoBin, "build", "-o", wasm, ".")
		}
		if err != nil {
			return err
		}
		raw, gz, err := measure(filepath.Join(d.Path, wasm))
		if err != nil {
			return err
		}
		fmt.Fprint(os.Stderr, sizeLine(filepath.Join(d.Path, wasm), raw, gz))
	}
	return nil
}

// stale reports whether output is missing or older than input.
func stale(input, output string) bool {
	in, err := os.Stat(input)
	if err != nil {
		return false
	}
	o, err := os.Stat(output)
	return err != nil || o.ModTime().Before(in.ModTime())
}

// Exec runs the directory's binary under fnox with args, replacing this
// process so signals reach it directly; or, as a Worker, wrangler dev in the
// directory.
func Exec(path string, asWorker bool, env string, args []string) error {
	d, err := Inspect(path)
	if err != nil {
		return err
	}
	if asWorker {
		if !d.Wrangler {
			return fmt.Errorf("%s has no wrangler.toml; it is not a Worker", d.Path)
		}
		return execIn(d.Path, WranglerBin, append([]string{"dev", "--env", env}, args...)...)
	}
	if d.WasmOnly {
		return fmt.Errorf("%s builds only a Worker; run it with --worker", d.Path)
	}
	return execIn(d.Root, FnoxBin, append([]string{"exec", "--", filepath.Join(BinDir, d.Name)}, args...)...)
}

// Check checks the directory: gsx formatting, go vet and go test (vet only,
// for wasm, where tests cannot run), a Worker's smoke round trip on workerd
// requesting reqPath, and the browser probe when the directory ships one.
func Check(out io.Writer, path, reqPath, expect string) error {
	d, err := Inspect(path)
	if err != nil {
		return err
	}
	if d.GSX {
		if err := run(out, d.Path, nil, GoBin, "tool", "gsx", "fmt", "-l", "."); err != nil {
			return fmt.Errorf("unformatted .gsx above; fix with: go tool gsx fmt -w %s", d.Path)
		}
	}
	// A directory with a Worker has a wasm target; one with a native main has
	// that too. Vet every target it has, test where tests can run.
	if !d.WasmOnly {
		if err := run(out, d.Path, nil, GoBin, "vet", "./..."); err != nil {
			return err
		}
		if err := run(out, d.Path, nil, GoBin, "test", "./..."); err != nil {
			return err
		}
	}
	if d.WasmOnly || d.Wrangler {
		if err := run(out, d.Path, []string{GoOSEnv + "=js", GoArchEnv + "=wasm"}, GoBin, "vet", "./..."); err != nil {
			return err
		}
	}
	if d.Wrangler {
		if err := cloudflare.Smoke(out, d.Path, "", reqPath, expect, 3*time.Minute); err != nil {
			return err
		}
	}
	if script := filepath.Join(d.Path, "tools", "browser-check.mjs"); exists(script) {
		if err := probe(out, filepath.Join(BinDir, d.Name), script, reqPath); err != nil {
			return err
		}
	}
	return nil
}

// Usage is what dev prints for the stage verbs. It is markdown in a file
// beside this one, not a string const: a Go raw string is backtick-delimited,
// so it can never hold the inline code that keeps `<dir>` from reaching a
// markdown renderer as an HTML tag.

//go:embed usage.md
var Usage string

// EnvFlag, CheckFlags and the rest are each verb's flags, registered in one
// place. main.go hands them to cli, which renders the signature from them;
// Run calls the same function and reads the values back by name. One
// registration, so a flag cannot exist without appearing in the manual.

// EnvFlag is the wrangler environment, taken by wasm and workerd.
func EnvFlag(fs *flag.FlagSet) {
	fs.String("env", "", "wrangler environment `NAME`")
}

// CheckFlags are what `check` requests and expects of the smoke check.
func CheckFlags(fs *flag.FlagSet) {
	fs.String("path", "/", "the path `P` the smoke check and the browser probe request")
	fs.String("expect", "", "the `TEXT` the smoke check's body must contain")
}

// Run is every stage verb. DIR comes first; flags may follow anywhere.
func Run(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	switch verb {
	case "build":
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		return Build(stdout, dir, false, "")
	case "wasm":
		EnvFlag(fs)
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		return Build(stdout, dir, true, cli.Value(fs, "env"))
	case "check":
		CheckFlags(fs)
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		return Check(stdout, dir, cli.Value(fs, "path"), cli.Value(fs, "expect"))
	case "run":
		dir, rest, err := cli.DirAnd(fs, args, -1)
		if err != nil {
			return err
		}
		return Exec(dir, false, "", rest)
	case "workerd":
		EnvFlag(fs)
		dir, rest, err := cli.DirAnd(fs, args, -1)
		if err != nil {
			return err
		}
		return Exec(dir, true, cli.Value(fs, "env"), rest)
	}
	return cli.Usagef("stage: unknown verb %q", verb)
}
