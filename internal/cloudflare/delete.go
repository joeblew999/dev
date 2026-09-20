package cloudflare

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/fnox"
)

// Delete removes a Worker and what wrangler provisioned for it. The Worker
// is the one dir's config deploys env to, suffix included, or name when
// given (for one a rename or an old config left behind). The KV namespaces
// go too, found by the title wrangler gives what it provisions,
// <worker>-<binding> in lowercase, so a namespace made by hand is never
// touched. It says what will go and asks, unless yes.
func Delete(stdin io.Reader, out io.Writer, dir, env, name string, yes bool) error {
	cfg, err := readWrangler(filepath.Join(dir, ConfigFile))
	if err != nil {
		return err
	}
	name = cmp.Or(name, workerName(cfg, env))
	all, err := namespaces(dir)
	if err != nil {
		return err
	}
	var doomed []namespace
	for _, title := range provisionedTitles(cfg.kvBindings(env), name) {
		for _, ns := range all {
			if ns.Title == title {
				doomed = append(doomed, ns)
			}
		}
	}
	fmt.Fprintf(out, "will delete the Worker %s\n", name)
	for _, ns := range doomed {
		fmt.Fprintf(out, "will delete its KV namespace %s (%s) and everything in it\n", ns.Title, ns.ID)
	}
	if len(doomed) == 0 {
		fmt.Fprintln(out, "no KV namespace wrangler provisioned for it; one made by hand stays")
	}
	if !yes && !cli.Confirm(stdin, out, "delete? [y/N] ") {
		return fmt.Errorf("not deleted (pass --yes to skip the question)")
	}
	// Asked with both streams so wrangler's own words reach the reader.
	// Without them a Worker that is not there failed as a bare exit status,
	// which says nothing about whether it was missing, not yours, or a
	// credentials problem.
	//
	// Fly's side of this goes further: it asks whether the app exists first,
	// so destroying what is already gone says so and succeeds, which is what
	// a cleanup run needs the second time. The same is not done here because
	// it would mean matching wrangler's wording for an absent Worker, and
	// there is no wrangler.toml and no Cloudflare account on this machine to
	// find out what that wording is. Guessing it is how the Fly version came
	// to read an empty list as proof of absence.
	said, err := fnox.Ask(dir, "wrangler", "delete", "--name", name, "--force")
	fmt.Fprint(out, said)
	if err != nil {
		return fmt.Errorf("wrangler delete %s failed: %w\n%s", name, err, cli.Indent(said))
	}
	for _, ns := range doomed {
		if err := fnox.Exec(dir, nil, out, "wrangler", "kv", "namespace", "delete", "--namespace-id", ns.ID); err != nil {
			return fmt.Errorf("deleting KV namespace %s (%s): %w", ns.Title, ns.ID, err)
		}
	}
	fmt.Fprintf(out, "deleted %s\n", name)
	return nil
}

// provisionedTitles is what wrangler calls the namespace it provisions for
// each binding of a Worker: the Worker's name, a dash, the binding in
// lowercase with dashes.
func provisionedTitles(bindings []kvBinding, worker string) []string {
	return cli.Map(bindings, func(b kvBinding) string {
		return worker + "-" + strings.ReplaceAll(strings.ToLower(b.Binding), "_", "-")
	})
}

type namespace struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// namespaces lists the account's KV namespaces through wrangler.
func namespaces(dir string) ([]namespace, error) {
	var buf bytes.Buffer
	if err := fnox.Exec(dir, nil, &buf, "wrangler", "kv", "namespace", "list"); err != nil {
		return nil, fmt.Errorf("wrangler kv namespace list failed: %w", err)
	}
	// wrangler prints its banner before the JSON; DecodeJSON starts at it.
	return cli.DecodeJSON[[]namespace]("wrangler's namespace list", buf.String())
}
