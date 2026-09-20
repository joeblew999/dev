// What CheckPinned reads, held to the shapes a real mise.toml uses.
package cli

import "testing"

// A pin may carry settings as well as a version: wrangler needs allow_builds
// before npm will install it, and then the [tools] line is an inline table.
// A hand-written line reader made the whole brace-blob the version, and a
// repo that pinned wrangler correctly was told it disagreed with a registry
// saying the same thing. It is a real TOML parser now, so what this holds is
// that the shapes a mise.toml actually uses come back right.
func TestAPinCanCarrySettingsAsWellAsAVersion(t *testing.T) {
	tools := miseTools(`
[tools]
go = "1.27.1"
wrangler = { version = "4.1.0", allow_builds = ["esbuild", "sharp"] }
"go:github.com/mibk/dupl" = "v1.1.0"   # a trailing comment
flyctl = "latest"

[tools.something]
setting = "not a pin"

[tasks.x]
run = "echo"
`)
	for name, want := range map[string]string{
		"go":                      "1.27.1",
		"wrangler":                "4.1.0",
		"go:github.com/mibk/dupl": "v1.1.0",
		"flyctl":                  "latest",
	} {
		if got := tools[name]; got != want {
			t.Errorf("%s = %q; want %q", name, got, want)
		}
	}
	// A sub-table is settings for a tool, not a pin, and a task is not a tool.
	if v, ok := tools["something"]; ok && v != "" {
		t.Errorf("[tools.something] read as a pin: %q", v)
	}
	if _, ok := tools["run"]; ok {
		t.Error("a line from [tasks.x] was read as a tool pin")
	}
}
