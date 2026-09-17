package release

import (
	"strings"
	"testing"
)

func TestGoreleaserConfigNamesTheCommandAndBinary(t *testing.T) {
	cfg := goreleaserConfig("acme", "cmd/api", "RWQpub")
	for _, want := range []string{"project_name: acme", "dir: cmd/api", "binary: acme", "goos: [linux, darwin, windows]", "CGO_ENABLED=0", "-X main.pubkey=RWQpub"} {
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

func TestCreateNamesEveryArtifactsDownloadURL(t *testing.T) {
	r := &release{slug: "acme/tool", name: "tool", bins: []string{"tool"}, skills: []string{"skill/tool=repo:skills/tool"}}
	got := strings.Join(r.createArgs("1.2.3", "abc", "v1.2.3", "", false, []string{".dist/tool_1.2.3_linux_amd64.tar.gz"}), " ")
	for _, want := range []string{
		"--url-base https://github.com/acme/tool/releases/download/v1.2.3/ ",
		"--notes-url https://github.com/acme/tool/releases/tag/v1.2.3 ",
		"--resource skill/tool=repo:skills/tool .dist/tool_1.2.3_linux_amd64.tar.gz",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("create args lack %q:\n%s", want, got)
		}
	}
	if !strings.Contains(strings.Join(r.createArgs("0.0.0-x", "abc", "v0.0.0-x", "k", true, nil), " "), "--key k --no-log") {
		t.Error("a snapshot did not sign with the throwaway key unlogged")
	}
}

// gsx and gsxui each ship two binaries from their own goreleaser config.
func TestBinariesComeFromTheRepoConfig(t *testing.T) {
	cfg := []byte("builds:\n  - id: gsx\n    binary: gsx\n  - id: tb\n    binary: \"gsx-typebundle\"\narchives:\n  - name_template: x\n")
	if got := strings.Join(binaries(cfg), ","); got != "gsx,gsx-typebundle" {
		t.Errorf("binaries = %q", got)
	}
	r := &release{slug: "acme/gsx", name: "gsx", bins: []string{"gsx", "gsx-typebundle"}}
	if got := strings.Join(r.createArgs("1.0.0", "c", "v1.0.0", "", false, nil), " "); !strings.Contains(got, "--bin gsx --bin gsx-typebundle") {
		t.Errorf("create args name one binary: %s", got)
	}
}
