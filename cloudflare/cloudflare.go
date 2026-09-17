// Package cloudflare is the Cloudflare Workers target: a Worker's URL for this
// clone, a deploy that leaves no personal value in git, its logs, a smoke
// round trip on workerd, and its secrets. Every verb takes the Worker's
// directory, the one holding its wrangler.toml; package app sends a directory
// here when it finds that file.
package cloudflare

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/joeblew999/dev/fnox"
	"github.com/joeblew999/dev/internal/cli"
)

// stdin is where delete's question is answered; a test replaces it.
var stdin io.Reader = os.Stdin

// Run is every Worker verb but wait. DIR comes first; flags may follow anywhere.
func Run(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	env := fs.String("env", "", "wrangler environment")
	switch verb {
	case "url":
		var deployed, refresh cli.Bool
		fs.Var(&deployed, "deployed", "the deployed Worker's URL; otherwise --local")
		fs.Var(&refresh, "refresh", "ask the API again instead of reading mise.local.toml")
		local := fs.String("local", "", "what to print when not --deployed")
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		u, err := URL(dir, *env, bool(deployed), *local, bool(refresh))
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, u)
		return nil
	case "deploy":
		wait := fs.String("wait", "", "path to wait for a 200 on after deploying, e.g. /health")
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		if err := Deploy(stdout, dir, *env); err != nil {
			return err
		}
		if *wait == "" {
			return nil
		}
		u, err := URL(dir, *env, true, "", false)
		if err != nil {
			return err
		}
		return Wait(stdout, u+*wait, 2*time.Minute)
	case "logs":
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		return Logs(dir, *env)
	case "delete":
		name := fs.String("name", "", "the Worker to delete (default: the one the config deploys env to)")
		var yes cli.Bool
		fs.Var(&yes, "yes", "delete without asking")
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		return Delete(stdin, stdout, dir, *env, *name, bool(yes))
	case "smoke":
		path := fs.String("path", "/", "what to request")
		expect := fs.String("expect", "", "text the body must contain")
		timeout := fs.Duration("timeout", 3*time.Minute, "how long wrangler dev may take to start")
		dir, _, err := cli.DirAnd(fs, args, 0)
		if err != nil {
			return err
		}
		return Smoke(stdout, dir, *env, *path, *expect, *timeout)
	}
	return cli.Usagef("cloudflare: unknown verb %q", verb)
}

// PutSecret is `wrangler secret put`, which reads the value on stdin and
// deploys a new version of the Worker with it. The Worker is named, so a
// developer's suffixed copy gets its own secrets.
func PutSecret(dir, env, name, value string) error {
	target, err := Name(dir, env)
	if err != nil {
		return err
	}
	return fnox.Exec(dir, strings.NewReader(value), io.Discard, "wrangler", "secret", "put", name, "--env", env, "--name", target)
}
