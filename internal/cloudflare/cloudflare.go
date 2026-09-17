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

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/fnox"
)

// stdin is where delete's question is answered; a test replaces it.
var stdin io.Reader = os.Stdin

// Run is every Worker verb but wait. DIR comes first; flags may follow anywhere.
// Run is every Worker verb but wait. cli has parsed DIR and the flags before
// this is reached, so each case is the call it makes and nothing else.
func Run(verb string, c cli.Call) error {
	switch verb {
	case "url":
		u, err := URL(c.Dir, c.Value("env"), c.Given("deployed"), c.Value("local"), c.Given("refresh"))
		if err != nil {
			return err
		}
		fmt.Fprintln(c.Stdout, u)
		return nil
	case "deploy":
		if err := Deploy(c.Stdout, c.Dir, c.Value("env")); err != nil {
			return err
		}
		if c.Value("wait") == "" {
			return nil
		}
		u, err := URL(c.Dir, c.Value("env"), true, "", false)
		if err != nil {
			return err
		}
		return Wait(c.Stdout, u+c.Value("wait"), 2*time.Minute)
	case "logs":
		return Logs(c.Dir, c.Value("env"))
	case "delete":
		return Delete(c.Stdin, c.Stdout, c.Dir, c.Value("env"), c.Value("name"), c.Given("yes"))
	case "smoke":
		d, _ := time.ParseDuration(c.Value("timeout"))
		return Smoke(c.Stdout, c.Dir, c.Value("env"), c.Value("path"), c.Value("expect"), d)
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
