// Package fly is the Fly.io target: an app's URL for this clone, its deploy,
// its logs and its secrets. Every verb takes the app's directory, the one
// holding its fly.toml; package app sends a directory here when it finds that
// file. Deploys run from the repo root, which is the Docker build context, as
// the upstream gsx repos do; the Dockerfile is relative to fly.toml.
package fly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/fnox"
	"github.com/joeblew999/dev/internal/suffix"
)

// ConfigFile is the file whose presence makes a directory a Fly app.
const ConfigFile = "fly.toml"

const (
	// FlyctlBin is the CLI every Fly verb runs through.
	FlyctlBin = "flyctl"
	// FnoxBin is the wrapper that supplies the account's credentials.
	FnoxBin = "fnox"
	// OrgEnv names the org a new app is created in.
	OrgEnv = "FLY_ORG"
)

// Run is every Fly verb but wait. DIR comes first; flags may follow anywhere,
// and for deploy everything after a bare -- goes to flyctl.
func Run(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	env := fs.String("env", "", "not a Fly concept; one fly.toml is one app")
	switch verb {
	case "url":
		var deployed, refresh cli.Bool
		fs.Var(&deployed, "deployed", "the deployed app's URL; otherwise --local")
		fs.Var(&refresh, "refresh", "accepted for symmetry with a Worker; a Fly URL is never cached")
		local := fs.String("local", "", "what to print when not --deployed")
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		if err := noEnv(dir, *env); err != nil {
			return err
		}
		if !deployed {
			fmt.Fprintln(stdout, *local)
			return nil
		}
		u, err := URL(dir)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, u)
		return nil
	case "deploy":
		waitPath := fs.String("wait", "", "path to wait for a 200 on after deploying, e.g. /health")
		dir, extra, err := cli.DirAnd(fs, args, -1)
		if err != nil {
			return err
		}
		if err := noEnv(dir, *env); err != nil {
			return err
		}
		if err := Deploy(stdout, dir, extra); err != nil {
			return err
		}
		if *waitPath == "" {
			return nil
		}
		u, err := URL(dir)
		if err != nil {
			return err
		}
		return Wait(stdout, u+*waitPath, 2*time.Minute)
	case "logs":
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		if err := noEnv(dir, *env); err != nil {
			return err
		}
		return Logs(dir)
	case "delete":
		name := fs.String("name", "", "the app to destroy (default: the one fly.toml names, with the suffix)")
		var yes cli.Bool
		fs.Var(&yes, "yes", "destroy without asking")
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		if err := noEnv(dir, *env); err != nil {
			return err
		}
		return Destroy(stdin, stdout, dir, *name, bool(yes))
	case "smoke":
		return fmt.Errorf("smoke runs a Worker on local workerd; a Fly app has no local runtime here. dev check DIR tests it, and dev deploy DIR --wait PATH proves it online")
	}
	return cli.Usagef("fly: unknown verb %q", verb)
}

func noEnv(dir, env string) error {
	if env != "" {
		return fmt.Errorf("a Fly app has no environments (--env %q): %s deploys one app; a second app is a second directory", env, filepath.Join(dir, ConfigFile))
	}
	return nil
}

// App is the app dir's fly.toml deploys, with the developer's suffix.
func App(dir string) (string, error) {
	var cfg struct {
		App string `toml:"app"`
	}
	path := filepath.Join(dir, ConfigFile)
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if cfg.App == "" {
		return "", fmt.Errorf("%s has no app; add: app = \"<name>\"", path)
	}
	return suffix.Apply(cfg.App), nil
}

// URL is the app's fly.dev address.
func URL(dir string) (string, error) {
	app, err := App(dir)
	if err != nil {
		return "", err
	}
	return "https://" + app + ".fly.dev", nil
}

// Deploy is `flyctl deploy` for the app in dir, run from the repo root as the
// build context, under fnox for FLY_API_TOKEN. A suffixed app is named
// explicitly; the committed fly.toml keeps the shared name. extra goes to
// flyctl as typed (--ha=false, --remote-only, ...).
func Deploy(out io.Writer, dir string, extra []string) error {
	if err := installed(); err != nil {
		return err
	}
	app, err := App(dir)
	if err != nil {
		return err
	}
	if err := ensureApp(out, app); err != nil {
		return err
	}
	args := []string{FlyctlBin, "deploy", "--config", filepath.Join(dir, ConfigFile)}
	if suffix.Set() {
		args = append(args, "--app", app)
	}
	args = append(args, extra...)
	args = append(args, ".")
	if err := fnox.Exec(".", nil, out, args...); err != nil {
		return fmt.Errorf("flyctl deploy of %s failed: %w. It needs FLY_API_TOKEN in fnox (a deploy token from: flyctl tokens create deploy), or a login from: flyctl auth login; and the app must exist: flyctl apps create %s", app, err, app)
	}
	return nil
}

// stdin is where delete's question is answered; a test replaces it.
var stdin io.Reader = os.Stdin

// Destroy removes the app dir's fly.toml names, suffix included, or name
// when given, with its machines and volumes. It says so and asks, unless yes.
func Destroy(stdin io.Reader, out io.Writer, dir, name string, yes bool) error {
	if err := installed(); err != nil {
		return err
	}
	if name == "" {
		app, err := App(dir)
		if err != nil {
			return err
		}
		name = app
	}
	fmt.Fprintf(out, "will destroy the Fly app %s, its machines and volumes\n", name)
	if !yes && !cli.Confirm(stdin, out, "destroy? [y/N] ") {
		return fmt.Errorf("not destroyed (pass --yes to skip the question)")
	}
	if err := fnox.Exec(".", nil, out, FlyctlBin, "apps", "destroy", name, "--yes"); err != nil {
		return fmt.Errorf("flyctl apps destroy %s failed: %w", name, err)
	}
	fmt.Fprintf(out, "destroyed %s\n", name)
	return nil
}

// ensureApp creates the app when the account does not have it, since
// flyctl deploy will not: a developer's suffixed copy, or a fork whose
// upstream owns the committed name (Fly app names are global). The org is
// FLY_ORG when set, otherwise flyctl's default, the personal one.
func ensureApp(out io.Writer, app string) error {
	var list bytes.Buffer
	if err := fnox.Exec(".", nil, &list, FlyctlBin, "apps", "list", "--json"); err != nil {
		return fmt.Errorf("flyctl apps list failed: %w. It needs FLY_API_TOKEN in fnox (a token from: flyctl tokens create org), or a login from: flyctl auth login", err)
	}
	var apps []struct {
		Name string `json:"Name"`
	}
	text := list.String()
	if i := strings.Index(text, "["); i >= 0 {
		text = text[i:]
	}
	if err := json.Unmarshal([]byte(text), &apps); err != nil {
		return fmt.Errorf("reading flyctl's app list: %w", err)
	}
	for _, a := range apps {
		if a.Name == app {
			return nil
		}
	}
	args := []string{FlyctlBin, "apps", "create", app}
	if org := os.Getenv(OrgEnv); org != "" {
		args = append(args, "--org", org)
	}
	fmt.Fprintf(out, "creating the Fly app %s, which the account does not have yet\n", app)
	if err := fnox.Exec(".", nil, out, args...); err != nil {
		return fmt.Errorf("flyctl apps create %s failed: %w (a name is global across Fly; DEPLOY_SUFFIX gives this copy its own, FLY_ORG the org)", app, err)
	}
	return nil
}

// Logs streams the deployed app's logs in the foreground until interrupted.
func Logs(dir string) error {
	if err := installed(); err != nil {
		return err
	}
	app, err := App(dir)
	if err != nil {
		return err
	}
	cmd := exec.Command(FnoxBin, "exec", "--", FlyctlBin, "logs", "--app", app)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// PutSecret is `flyctl secrets import`, which reads NAME=VALUE lines on stdin
// and releases the app with them, so the value is never an argument.
func PutSecret(dir, name, value string) error {
	if err := installed(); err != nil {
		return err
	}
	app, err := App(dir)
	if err != nil {
		return err
	}
	return fnox.Exec(".", strings.NewReader(name+"="+value+"\n"), io.Discard, FlyctlBin, "secrets", "import", "--app", app)
}

func installed() error {
	if _, err := lookPath(FlyctlBin); err != nil {
		return fmt.Errorf("flyctl is not installed; add to mise.toml under [tools]: flyctl = \"latest\", then: mise install")
	}
	return nil
}

// The seams tests replace. wait is the same steady-200 poll a Worker gets;
// package app wires it, since this package must not import that one.
var (
	lookPath = exec.LookPath
	Wait     func(out io.Writer, url string, timeout time.Duration) error
)
