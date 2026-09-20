// Package cloudflare is the Cloudflare Workers target: a Worker's URL for this
// clone, a deploy that leaves no personal value in git, its logs, a smoke
// round trip on workerd, and its secrets. Every verb takes the Worker's
// directory, the one holding its wrangler.toml; package app sends a directory
// here when it finds that file.
package cloudflare

import (
	"io"
	"strings"

	"github.com/joeblew999/dev/internal/fnox"
)

// What this package is about, named here rather than in whichever file
// happened to need them first. Every other file in the package reads them
// from here.
const (
	// ConfigFile is the file whose presence makes a directory a Worker.
	ConfigFile = "wrangler.toml"
	// WranglerBin is the CLI every Worker verb runs through.
	WranglerBin = "wrangler"
)

// PutSecret is `wrangler secret put`, which reads the value on stdin and
// deploys a new version of the Worker with it. The Worker is named, so a
// developer's suffixed copy gets its own secrets.
func PutSecret(dir, env, name, value string) error {
	target, err := Name(dir, env)
	if err != nil {
		return err
	}
	return fnox.Exec(dir, strings.NewReader(value), io.Discard, WranglerBin, "secret", "put", name, "--env", env, "--name", target)
}
