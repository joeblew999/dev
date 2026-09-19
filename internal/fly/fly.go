// Package fly is the Fly.io target: an app's URL for this clone, its deploy,
// its logs and its secrets. Every verb takes the app's directory, the one
// holding its fly.toml; package app sends a directory here when it finds that
// file. Deploys run from the repo root, which is the Docker build context, as
// the upstream gsx repos do; the Dockerfile is relative to fly.toml.
package fly

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
	"github.com/joeblew999/dev/internal/conf"
	"github.com/joeblew999/dev/internal/fnox"
	"github.com/joeblew999/dev/internal/suffix"
)

// ConfigFile is the file whose presence makes a directory a Fly app.
const ConfigFile = "fly.toml"

const (
	// FlyctlBin is the CLI every Fly verb runs through.
	FlyctlBin = "flyctl"
	// OrgEnv names the org a new app is created in.
	OrgEnv = "FLY_ORG"
)

// NoEnv refuses --env: a Fly app has one environment, and a second app is a
// second directory. app checks this before any Fly verb runs.
func NoEnv(dir, env string) error {
	if env != "" {
		return fmt.Errorf("a Fly app has no environments (--env %q): %s deploys one app; a second app is a second directory", env, filepath.Join(dir, ConfigFile))
	}
	return nil
}

// App is the app dir's fly.toml deploys, with the developer's suffix.
func App(dir string) (string, error) {
	path := filepath.Join(dir, ConfigFile)
	cfg, err := conf.Load[struct {
		App string `toml:"app"`
	}](path)
	if err != nil {
		return "", err
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
	app, err := ready(dir)
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

// Destroy removes the app dir's fly.toml names, suffix included, or name
// when given, with its machines and volumes. It says so and asks, unless yes.
func Destroy(stdin io.Reader, out io.Writer, dir, name string, yes bool) error {
	if name == "" {
		app, err := ready(dir)
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
	type flyApp struct {
		Name string `json:"Name"`
	}
	apps, err := cli.DecodeJSON[[]flyApp]("flyctl's app list", list.String())
	if err != nil {
		return err
	}
	if slices.ContainsFunc(apps, func(a flyApp) bool { return a.Name == app }) {
		return nil
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
	app, err := ready(dir)
	if err != nil {
		return err
	}
	return tool.Attached("", fnox.Bin, "exec", "--", FlyctlBin, "logs", "--app", app)
}

// PutSecret is `flyctl secrets import`, which reads NAME=VALUE lines on stdin
// and releases the app with them, so the value is never an argument.
func PutSecret(dir, name, value string) error {
	app, err := ready(dir)
	if err != nil {
		return err
	}
	return fnox.Exec(".", strings.NewReader(name+"="+value+"\n"), io.Discard, FlyctlBin, "secrets", "import", "--app", app)
}

// ready is what every verb needs before it can do anything: flyctl on PATH,
// and the app this directory names. Four verbs opened with the same six
// lines, which is four places to forget one of them.
func ready(dir string) (string, error) {
	if err := installed(); err != nil {
		return "", err
	}
	return App(dir)
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
)
