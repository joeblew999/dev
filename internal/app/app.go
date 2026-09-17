// Package app is what a command becomes once deployed, on whichever cloud its
// directory names: a wrangler.toml means Cloudflare Workers, a fly.toml means
// Fly. The verbs are the same for both; this package reads the directory and
// hands over to the target, so a task never says which cloud, and secrets go
// the same way.
package app

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/cloudflare"
	"github.com/joeblew999/dev/internal/fly"
)

// Usage is what dev prints for these verbs. It is markdown in a file beside
// this one, not a string const: a Go raw string is backtick-delimited, so it
// can never hold the inline code that keeps a `<placeholder>` from reaching a
// markdown renderer as an HTML tag.

//go:embed usage.md
var Usage string

func init() { fly.Wait = cloudflare.Wait }

// Run is every deployed-app verb. DIR comes first; the target's own Run
// reads the flags.
// The deploy verbs' flags. Both clouds register the same ones, checked verb
// by verb, with one exception: a Worker's smoke takes --path, --expect and
// --timeout and a Fly app's takes none, because only the Worker is run
// locally under workerd. The signature shows the Worker's, which is the
// larger set, and smoke's description says so.
//
// This is the one place a signature cannot be the whole truth: which cloud a
// verb is talking to is read from DIR, so it is not known until the verb
// runs. `<verb> DIR --help` resolves the directory first and prints that
// cloud's flags.

// EnvFlag is the wrangler environment, taken by every deploy verb.
func EnvFlag(fs *flag.FlagSet) {
	fs.String("env", "", "wrangler environment `NAME`")
}

// URLFlags are what `url` takes.
func URLFlags(fs *flag.FlagSet) {
	EnvFlag(fs)
	fs.Var(new(cli.Bool), "deployed", "the deployed app's URL; otherwise --local")
	fs.Var(new(cli.Bool), "refresh", "ask the API again instead of reading mise.local.toml")
	fs.String("local", "", "the `URL` to print when not --deployed")
}

// DeployFlags are what `deploy` takes.
func DeployFlags(fs *flag.FlagSet) {
	EnvFlag(fs)
	fs.String("wait", "", "the `PATH` to wait for a 200 on after deploying, e.g. /health")
}

// SmokeFlags are what `smoke` takes against a Worker; a Fly app takes none.
func SmokeFlags(fs *flag.FlagSet) {
	EnvFlag(fs)
	fs.String("path", "/", "the `P` to request")
	fs.String("expect", "", "the `TEXT` the body must contain")
	fs.Duration("timeout", 3*time.Minute, "how `LONG` wrangler dev may take to start")
}

// DeleteFlags are what `delete` takes.
func DeleteFlags(fs *flag.FlagSet) {
	EnvFlag(fs)
	fs.String("name", "", "the `APP` to remove (default: the one the config deploys env to)")
	fs.Var(new(cli.Bool), "yes", "remove without asking")
}

// WaitFlags are what `wait` takes.
func WaitFlags(fs *flag.FlagSet) {
	fs.Duration("timeout", 2*time.Minute, "how `LONG` to keep trying")
}

// Each verb is the work it does, and which cloud does it comes from the
// directory. cli has already parsed DIR and the flags, so these are the
// dispatch and nothing else.

func URLVerb(c cli.Call) error    { return to(c, "url") }
func DeployVerb(c cli.Call) error { return to(c, "deploy") }
func LogsVerb(c cli.Call) error   { return to(c, "logs") }
func SmokeVerb(c cli.Call) error  { return to(c, "smoke") }
func DeleteVerb(c cli.Call) error { return to(c, "delete") }

// WaitVerb is the one verb here that talks to no cloud: it polls a URL.
func WaitVerb(c cli.Call) error {
	if len(c.Args) != 1 {
		return c.Usagef("needs exactly one URL")
	}
	d, _ := time.ParseDuration(c.Value("timeout"))
	return cloudflare.Wait(c.Stdout, c.Args[0], d)
}

// to sends a verb to whichever cloud the directory deploys to.
func to(c cli.Call, verb string) error {
	target, err := Target(c.Dir)
	if err != nil {
		return err
	}
	if target == "fly" {
		return fly.Run(verb, c)
	}
	return cloudflare.Run(verb, c)
}

// Target names the cloud dir deploys to, "cloudflare" or "fly", from the
// config file it holds.
func Target(dir string) (string, error) {
	cf, fl := exists(filepath.Join(dir, cloudflare.ConfigFile)), exists(filepath.Join(dir, fly.ConfigFile))
	switch {
	case cf && fl:
		return "", fmt.Errorf("%s has both wrangler.toml and fly.toml; a directory deploys to one cloud, so split it in two", dir)
	case cf:
		return "cloudflare", nil
	case fl:
		return "fly", nil
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", dir)
	}
	return "", fmt.Errorf("%s has no wrangler.toml or fly.toml, so nothing deploys it; add one beside its main.go", dir)
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// Name is what the app in dir is called on its cloud, suffix included.
func Name(dir, env string) (string, error) {
	target, err := Target(dir)
	if err != nil {
		return "", err
	}
	if target == "fly" {
		return fly.App(dir)
	}
	return cloudflare.Name(dir, env)
}

// PutSecret gives the deployed app in dir one secret, through its cloud's own
// CLI, the value on stdin and never an argument.
func PutSecret(dir, env, name, value string) error {
	target, err := Target(dir)
	if err != nil {
		return err
	}
	if target == "fly" {
		return fly.PutSecret(dir, name, value)
	}
	return cloudflare.PutSecret(dir, env, name, value)
}
