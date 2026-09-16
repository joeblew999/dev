package release

import (
	"strings"
	"testing"
)

func TestGoreleaserConfigNamesTheCommandAndBinary(t *testing.T) {
	cfg := goreleaserConfig("acme", "cmd/api")
	for _, want := range []string{"project_name: acme", "dir: cmd/api", "binary: acme", "goos: [linux, darwin, windows]", "CGO_ENABLED=0"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config lacks %q:\n%s", want, cfg)
		}
	}
}
