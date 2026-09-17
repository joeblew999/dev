// Package app is what a command becomes once deployed, on whichever cloud its
// directory names: a wrangler.toml means Cloudflare Workers, a fly.toml means
// Fly. The verbs are the same for both; this package reads the directory and
// hands over to the target, so a task never says which cloud, and secrets go
// the same way.
package app

import (
	_ "embed"
	"fmt"
	"io"
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
func Run(verb string, args []string, stdout, stderr io.Writer) error {
	if verb == "wait" {
		fs := cli.Flags(verb, stderr)
		timeout := fs.Duration("timeout", 2*time.Minute, "how long to keep trying")
		if err := fs.Parse(args); err != nil {
			return cli.Usagef("wait: %v", err)
		}
		if fs.NArg() != 1 {
			return cli.Usagef("wait needs exactly one URL")
		}
		return cloudflare.Wait(stdout, fs.Arg(0), *timeout)
	}
	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		return cli.Usagef("%s: the directory comes first", verb)
	}
	target, err := Target(args[0])
	if err != nil {
		return err
	}
	if target == "fly" {
		return fly.Run(verb, args, stdout, stderr)
	}
	return cloudflare.Run(verb, args, stdout, stderr)
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
