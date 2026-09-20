// Package fly is the Fly.io target: an app's URL for this clone, its deploy,
// its logs and its secrets. Every verb takes the app's directory, the one
// holding its fly.toml; package app sends a directory here when it finds that
// file. Deploys run from the repo root, which is the Docker build context, as
// the upstream gsx repos do; the Dockerfile is relative to fly.toml.
package fly

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
		return fmt.Errorf("flyctl deploy of %s failed: %w — %s", app, err, credentials(app))
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
	// Asked before the question is put, because there is no sense asking
	// someone to confirm destroying what is not there — and because an
	// absent app is how this ends the second time a cleanup runs.
	switch err := appStatus(name); {
	case errors.Is(err, errNoSuchApp):
		fmt.Fprintf(out, "there is no Fly app %s; nothing to destroy\n", name)
		return nil
	case err != nil:
		return err
	}
	fmt.Fprintf(out, "will destroy the Fly app %s, its machines and volumes\n", name)
	if !yes && !cli.Confirm(stdin, out, "destroy? [y/N] ") {
		return fmt.Errorf("not destroyed (pass --yes to skip the question)")
	}
	said, err := fnox.Ask(".", FlyctlBin, "apps", "destroy", name, "--yes")
	fmt.Fprint(out, said)
	if err != nil {
		return fmt.Errorf("flyctl apps destroy %s failed: %w", name, err)
	}
	fmt.Fprintf(out, "destroyed %s\n", name)
	return nil
}

// ensureApp creates the app when there is none, since flyctl deploy will not:
// a developer's suffixed copy, or a fork whose upstream owns the committed
// name — Fly app names are global. The org is FLY_ORG when set, otherwise
// flyctl's default, the personal one.
//
// It asks about this one app rather than listing them all. Listing answers a
// different question: `flyctl apps list` returned an empty array here for a
// token that could none the less resolve an org and validate a name, so "not
// in the list" was read as "does not exist", and deploying a live app tried to
// create it. A token scoped with `flyctl tokens create org` — which this very
// function used to recommend — is one that can deploy and cannot enumerate.
//
// Three answers, not two. The app is there; the app is nowhere, so make it;
// or the name is taken and these credentials cannot see it, which is the case
// that used to arrive as a validation error from Fly with no explanation of
// what to do.
func ensureApp(out io.Writer, app string) error {
	switch err := appStatus(app); {
	case err == nil:
		return nil
	case !errors.Is(err, errNoSuchApp):
		return err
	}
	args := []string{FlyctlBin, "apps", "create", app}
	if org := os.Getenv(OrgEnv); org != "" {
		args = append(args, "--org", org)
	}
	fmt.Fprintf(out, "creating the Fly app %s, which these credentials do not have\n", app)
	said, err := fnox.Ask(".", args...)
	fmt.Fprint(out, said)
	if err != nil {
		if strings.Contains(said, "already been taken") {
			return fmt.Errorf("the Fly app %s exists and cannot be seen from here, so it can be neither deployed to nor created — %s; or give this copy its own name with DEPLOY_SUFFIX", app, credentials(app))
		}
		return fmt.Errorf("flyctl apps create %s failed: %w (a name is global across Fly; DEPLOY_SUFFIX gives this copy its own, FLY_ORG the org)", app, err)
	}
	return nil
}

// errNoSuchApp is Fly saying this account has no app of that name, which is
// the one answer that means "create it".
var errNoSuchApp = errors.New("no such app")

// appStatus asks Fly about one app. A missing app is errNoSuchApp; anything
// else is a real failure and says what to do about it.
func appStatus(app string) error {
	said, err := fnox.Ask(".", FlyctlBin, "status", "--app", app, "--json")
	// What flyctl printed decides, not whether it exited zero and not what
	// its prose says. --json means an app that is there comes back as an
	// object naming itself, and that is the only evidence taken for yes:
	// matching a sentence is a dependency on wording that upstream is free to
	// change in a patch release, and this file has already been wrong twice
	// about what flyctl says and where it says it.
	if status, jsonErr := cli.DecodeJSON[struct {
		Name string `json:"Name"`
	}]("flyctl status", said); jsonErr == nil && status.Name != "" {
		return nil
	}
	if err == nil {
		return nil
	}
	// The sentence is still read, but only to tell a plain missing app from a
	// real failure — never to decide that one is there. Fly says it on
	// stderr, which is why the answer is asked for with both streams.
	if strings.Contains(said, "Could not find App") {
		return errNoSuchApp
	}
	return fmt.Errorf("flyctl could not say whether the app %s is there: %w — %s", app, err, credentials(app))
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

// Scaffold is a fly.toml worth having: the settings a production HTTP service
// wants, each with the reason it is there, so a reader can delete what does
// not apply rather than wonder what any of it does.
//
// Two are chosen against Fly's own recommendation, deliberately.
// min_machines_running is 0 where the docs say 1, because this stack deploys
// many small apps and a developer's own suffixed copy of each, and a machine
// held up for something nobody is looking at is a bill. A repo that cannot
// afford a cold start sets it to 1, and the file says so.
//
// The health check matters beyond Fly: `deploy --wait PATH` and `dev smoke`
// both ask whether the thing is actually answering, and a service Fly does
// not check is one Fly will route to before it is ready.
func Scaffold(dir, name string) string {
	return "# Written by `dev deploy --to fly` because " + dir + " had no " + ConfigFile + ".\n" +
		"# It is the convention, not a ceiling: edit it, commit it, it is yours.\n" +
		"app = " + strconv.Quote(name) + "\n\n" +
		"[build]\n\n" +
		"[http_service]\n" +
		"  # The Dockerfile sets PORT and a Go main on this stack reads it.\n" +
		"  internal_port = 8080\n" +
		"  force_https = true\n" +
		"  # Stop when idle and start on a request. min_machines_running = 1\n" +
		"  # keeps one warm if a cold start costs more than the machine does.\n" +
		"  auto_stop_machines = \"stop\"\n" +
		"  auto_start_machines = true\n" +
		"  min_machines_running = 0\n\n" +
		"  # Requests rather than connections: one connection can carry many,\n" +
		"  # so counting connections lets a few clients look like no load.\n" +
		"  [http_service.concurrency]\n" +
		"    type = \"requests\"\n" +
		"    soft_limit = 200\n" +
		"    hard_limit = 250\n\n" +
		"  # Fly routes to a machine it believes is healthy, so without this it\n" +
		"  # routes to one that has not finished starting. grace_period has to\n" +
		"  # exceed the slowest start, or a slow boot reads as a dead machine.\n" +
		"  [[http_service.checks]]\n" +
		"    method = \"GET\"\n" +
		"    path = \"/\"\n" +
		"    grace_period = \"10s\"\n" +
		"    interval = \"30s\"\n" +
		"    timeout = \"5s\"\n\n" +
		"# The smallest machine that runs a Go binary comfortably. Raise it\n" +
		"# when something is actually slow, not before.\n" +
		"[[vm]]\n" +
		"  size = \"shared-cpu-1x\"\n" +
		"  memory = \"512mb\"\n\n" +
		"# One machine at a time, so a bad release never takes them all.\n" +
		"[deploy]\n" +
		"  strategy = \"rolling\"\n"
}

// tokenIdentity is how Fly names a scoped token rather than a person: `fly
// auth whoami` answers with a uuid at this domain.
const tokenIdentity = "@tokens.fly.io"

// whoami is who these credentials are, "" when Fly will not say.
func whoami() string {
	said, err := fnox.Ask(".", FlyctlBin, "auth", "whoami")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(said)
}

// credentials says who is being refused, and what kind of credential it is.
//
// Every way this package can be refused looks the same from outside —
// "unauthorized", an empty list, an app that cannot be found — and all of
// them have the same cause more often than not: FLY_API_TOKEN is a scoped
// token for some other app. A scoped token is not a smaller login, it is a
// key to one app, so it resolves an org, validates a name and refuses
// everything about any app but its own. Nothing in Fly's own errors says so.
//
// This turns that into a sentence. It costs one flyctl call, and only on the
// path where something has already gone wrong.
func credentials(app string) string {
	who := whoami()
	switch {
	case who == "":
		return "and Fly will not say who these credentials are, which usually means there is no FLY_API_TOKEN in fnox and no login either (flyctl auth login, or fnox set -g FLY_API_TOKEN)"
	case strings.Contains(who, tokenIdentity):
		// A token, not a person. Which is not the same as app-scoped: an org
		// token wears the same identity, and saying "a key to one app" was
		// wrong for exactly the token this was written against. What it can
		// reach is a question with an answer, so ask it rather than guess.
		where := "no organisation at all"
		if orgs := orgsSeen(); len(orgs) > 0 {
			where = "the " + cli.English(orgs) + " " + cli.Plural(len(orgs), "organisation")[2:]
		}
		return fmt.Sprintf("and these credentials are a Fly token (%s) that can reach %s, which %s is not in — use a token for the organisation that owns it (`flyctl tokens create org` from an account that can see it), or `flyctl auth login`", who, where, app)
	}
	return fmt.Sprintf("and these credentials are %s, which does not have it; check the account that owns %s", who, app)
}

// orgsSeen is the organisations these credentials can reach, which is the
// fact that decides whether an app is reachable at all. Empty when Fly will
// not say.
func orgsSeen() []string {
	said, err := fnox.Ask(".", FlyctlBin, "orgs", "list", "--json")
	if err != nil {
		return nil
	}
	// A map of slug to display name.
	orgs, err := cli.DecodeJSON[map[string]string]("flyctl orgs list", said)
	if err != nil {
		return nil
	}
	return cli.SortedKeys(orgs)
}
