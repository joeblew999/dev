// Package app is what a command becomes once deployed, on whichever cloud its
// directory names: a wrangler.toml means Cloudflare Workers, a fly.toml means
// Fly. The verbs are the same for both; this package reads the directory and
// hands over to the target, so a task never says which cloud, and secrets go
// the same way.
package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/joeblew999/dev/cloudflare"
	"github.com/joeblew999/dev/fly"
	"github.com/joeblew999/dev/internal/cli"
)

const Usage = `dev url DIR [--deployed[=BOOL]] [--env NAME] [--local URL] [--refresh]
    print the URL to talk to: the deployed app in DIR when --deployed, else
    --local (default empty). A Worker's needs the account's workers.dev
    subdomain: read once with the credentials in fnox, kept in gitignored
    mise.local.toml, --refresh asking again. A Fly app's is <app>.fly.dev.
dev deploy DIR [--env NAME] [--wait PATH] [-- FLAGS]
    deploy what DIR holds. A Worker deploys from a throwaway copy of its
    wrangler.toml, so the ids wrangler writes back never reach git, and says
    what was created. A Fly app deploys with the repo root as build context,
    FLAGS going to flyctl, created first when the account lacks it (FLY_ORG
    names the org). With --wait, wait until it answers 200 at PATH
dev logs DIR [--env NAME]
    stream the deployed app's logs (wrangler tail, flyctl logs)
dev smoke DIR [--env NAME] [--path P] [--expect TEXT] [--timeout DURATION]
    run a Worker on local workerd with wrangler dev, request P (default /),
    and fail unless it answers 200 with TEXT in the body
dev wait URL [--timeout DURATION]
    wait until URL answers 200 steadily
dev delete DIR [--env NAME] [--name APP] [--yes]
    remove the deployed app in DIR, or APP (one a rename or an old config left
    behind), and for a Worker the KV namespaces wrangler provisioned for it,
    titled <worker>-<binding>; a namespace made by hand stays. Says what will
    go and asks, unless --yes

Which cloud DIR deploys to is read from it: wrangler.toml means Cloudflare
Workers, fly.toml means Fly; --env is a wrangler environment. DEPLOY_SUFFIX
in gitignored mise.local.toml gives a developer their own copy of every app.
Run from the repo root; needs fnox, and wrangler or flyctl.
`

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
	cf, fl := exists(filepath.Join(dir, "wrangler.toml")), exists(filepath.Join(dir, "fly.toml"))
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
