// What CheckPinned reads, held to the shapes a real mise.toml uses.
package cli

import (
	"strings"
	"testing"
)

// A pin may carry settings as well as a version: wrangler needs
// allow_builds before npm will install it, and then the [tools] line is an
// inline table. Reading everything after the first = as the version made the
// whole brace-blob the version, and a repo that pinned wrangler correctly was
// told it disagreed with a registry saying the same thing.
func TestAPinCanCarrySettingsAsWellAsAVersion(t *testing.T) {
	for _, tc := range []struct{ line, want string }{
		{`go = "1.27.1"`, "1.27.1"},
		{`wrangler = { version = "latest", allow_builds = ["esbuild", "sharp"] }`, "latest"},
		{`"go:github.com/mibk/dupl" = "v1.1.0"   # a trailing comment`, "v1.1.0"},
		{`flyctl = "latest" # and one with no spaces#inside`, "latest"},
	} {
		key, value, _ := strings.Cut(tc.line, "=")
		value, _, _ = strings.Cut(value, "#")
		if got := version(value); got != tc.want {
			t.Errorf("%s -> %q; want %q", strings.TrimSpace(key), got, tc.want)
		}
	}
	// And through the reader itself, which is what CheckPinned calls.
	tools := miseTools("[tools]\n" + `wrangler = { version = "4.1.0", allow_builds = ["x"] }` + "\ngo = \"1.27.1\"\n[tasks.x]\nrun = \"echo\"\n")
	if tools["wrangler"] != "4.1.0" || tools["go"] != "1.27.1" {
		t.Errorf("read %v; want wrangler 4.1.0 and go 1.27.1", tools)
	}
	if _, ok := tools["run"]; ok {
		t.Error("a line from [tasks.x] was read as a tool pin")
	}
}
