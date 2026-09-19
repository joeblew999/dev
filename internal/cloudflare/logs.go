package cloudflare

import (
	"github.com/joeblew999/dev/cli/tool"
	"github.com/joeblew999/dev/internal/fnox"
)

// Logs streams the deployed Worker's logs, in the Worker's directory so
// wrangler finds its config, under fnox for the account. It runs in the
// foreground until interrupted.
func Logs(dir, env string) error {
	name, err := Name(dir, env)
	if err != nil {
		return err
	}
	return tool.Attached(dir, fnox.Bin, "exec", "--", WranglerBin, "tail", name, "--env", env)
}
