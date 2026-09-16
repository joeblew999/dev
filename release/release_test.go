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

func TestSnapshotVersionIsAlwaysSemver(t *testing.T) {
	for describe, want := range map[string]string{
		"v0.1.0":            "0.1.0",
		"v0.1.0-3-g2feb4c7": "0.1.0-3-g2feb4c7",
		"2feb4c7":           "0.0.0-2feb4c7",
		"":                  "0.0.0-",
	} {
		if v, tag := snapshotVersion(describe); v != want || tag != "v"+want {
			t.Errorf("snapshotVersion(%q) = %q, %q; want %q", describe, v, tag, want)
		}
	}
}
