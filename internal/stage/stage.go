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
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/cloudflare"
	"github.com/joeblew999/dev/internal/fnox"

	"github.com/joeblew999/dev/cli/tool"
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
	GoBin     = "go"
	NpmBin    = "npm"
	NodeBin   = "node"
	TinyGoBin = "tinygo"
	GsxTool   = "gsx"
)

// Env keys stage sets or reads.
const (
	GoOSEnv   = "GOOS"
	GoArchEnv = "GOARCH"
	GoPortEnv = "GO_PORT"
	ChromeEnv = "CHROME"
)

// run runs a tool in dir. Progress and the tool's own output go to stderr:
// stdout is for data, and a task that depends on a build may be piped.
//
// So it takes no writer, and for a while it took one it never used — an
// io.Writer threaded down from the verb through four callers, past a comment
// saying everything goes to stderr, and written to by nothing. A parameter
// the body never touches is a signature promising what the code does not do;
// `mise run unread` is what says so now.
func run(dir string, env []string, name string, args ...string) error {
	fmt.Fprintf(os.Stderr, "$ %s %s\n", name, strings.Join(args, " "))
	// Stdout to stderr on purpose: a build's output is progress, and stdout
	// belongs to whatever the task piping this is building.
	return tool.Cmd{Bin: name, Args: args, Dir: dir, Env: env}.Stream(os.Stderr)
}

// generate runs the gsx code generator when the directory has gsx sources.
// Both build paths need it and both had written the same four lines.
func generate(d Dir) error {
	if !d.GSX {
		return nil
	}
	return run(d.Path, nil, GoBin, "tool", "gsx", "generate", "-q")
}

// worker refuses a directory that is not one, in the words both verbs that
// ask were using.
func (d Dir) worker() error {
	if !d.Wrangler {
		return fmt.Errorf("%s has no wrangler.toml; it is not a Worker", d.Path)
	}
	return nil
}

// Build builds the directory's binary: npm and gsx first where they apply,
// then go build to .bin/<dir>. With asWorker it builds the Worker's wasm for
// env instead: the environment's main names the output directory, and one
// under build/tinygo is built with TinyGo, any other with Go.
//
// It takes no writer, unlike Check below, and the difference is real rather
// than an oversight: a build produces no data, only progress, and progress
// goes to stderr so that a task piping a build's stdout gets nothing from it.
// Check does write — a smoke round trip and a browser probe both report — so
// it takes the verb's stdout and this does not.
func Build(path string, asWorker bool, env string) error {
	d, err := Inspect(path)
	if err != nil {
		return err
	}
	if asWorker {
		return buildWorker(d, env)
	}
	if d.WasmOnly {
		return fmt.Errorf("%s builds only a Worker; build it with --worker", d.Path)
	}
	if d.NPM {
		if stale(filepath.Join(d.Path, "package-lock.json"), filepath.Join(d.Path, "node_modules", ".package-lock.json")) {
			if err := run(d.Path, nil, NpmBin, "ci"); err != nil {
				return err
			}
		}
		if err := run(d.Path, nil, NpmBin, "run", "build"); err != nil {
			return err
		}
	}
	if err := generate(d); err != nil {
		return err
	}
	if err := cli.Ignore(d.Root, BinDir); err != nil {
		return err
	}
	bin := filepath.Join(d.Root, BinDir, d.Name)
	if err := run(d.Path, nil, GoBin, "build", "-o", bin, "."); err != nil {
		return err
	}
	if d.CLI {
		// A verb-table command writes its own manual. Building it rewrites
		// every copy, so a changed verb reaches this repo's agents with no
		// step taken; go test holds them when nothing built.
		return run(d.Root, nil, bin, "skill")
	}
	return nil
}

func buildWorker(d Dir, env string) error {
	if err := d.worker(); err != nil {
		return err
	}
	if err := generate(d); err != nil {
		return err
	}
	var err error
	{
		main, ok := d.Mains[env]
		if !ok {
			return fmt.Errorf("%s/wrangler.toml has no environment %q", d.Path, env)
		}
		outDir := filepath.Dir(main)
		mode, goEnv := "go", []string{GoOSEnv + "=js", GoArchEnv + "=wasm"}
		if filepath.Base(outDir) == TinyGoBin {
			mode = TinyGoBin
		}
		if err := run(d.Path, nil, GoBin, "run", "github.com/syumai/workers-go/cmd/workers-assets-gen", "-mode="+mode, "-o", outDir); err != nil {
			return err
		}
		wasm := filepath.Join(outDir, "app.wasm")
		if mode == "tinygo" {
			err = run(d.Path, nil, TinyGoBin, "build", "-o", wasm, "-target", "wasm", "-no-debug", ".")
		} else {
			err = run(d.Path, goEnv, GoBin, "build", "-o", wasm, ".")
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
		return execIn(d.Path, cloudflare.WranglerBin, append([]string{"dev", "--env", env}, args...)...)
	}
	if d.WasmOnly {
		return fmt.Errorf("%s builds only a Worker; run it with --worker", d.Path)
	}
	return execIn(d.Root, fnox.Bin, append([]string{"exec", "--", filepath.Join(BinDir, d.Name)}, args...)...)
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
		if err := run(d.Path, nil, GoBin, "tool", "gsx", "fmt", "-l", "."); err != nil {
			return fmt.Errorf("unformatted .gsx above; fix with: go tool gsx fmt -w %s", d.Path)
		}
	}
	// A directory with a Worker has a wasm target; one with a native main has
	// that too. Vet every target it has, test where tests can run.
	if !d.WasmOnly {
		if err := run(d.Path, nil, GoBin, "vet", "./..."); err != nil {
			return err
		}
		if err := run(d.Path, nil, GoBin, "test", "./..."); err != nil {
			return err
		}
	}
	if d.WasmOnly || d.Wrangler {
		if err := run(d.Path, []string{GoOSEnv + "=js", GoArchEnv + "=wasm"}, GoBin, "vet", "./..."); err != nil {
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
