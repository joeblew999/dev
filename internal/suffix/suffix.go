// Package suffix gives a developer their own copy of every deployed app.
// With DEPLOY_SUFFIX=alice, an app called api deploys as api-alice, its URL,
// logs and secrets following. It lives in gitignored mise.local.toml, so the
// committed config stays everyone's, and an unset variable, as in CI, means
// the shared name.
package suffix

import (
	"os"
	"strings"
)

// Env is the variable's name.
const Env = "DEPLOY_SUFFIX"

// Set reports whether a suffix is in force.
func Set() bool { return os.Getenv(Env) != "" }

// Apply returns name with the developer's suffix, or name itself.
func Apply(name string) string {
	if s := strings.TrimPrefix(os.Getenv(Env), "-"); s != "" {
		return name + "-" + s
	}
	return name
}
