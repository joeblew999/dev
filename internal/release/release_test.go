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

// A tag this module path cannot carry must be refused before it is made.
// Go requires a module at v2 or above to say so in its path, and it does not
// warn: it refuses the consumer's require line, in their repo, after the tag
// is published and unfixable. This repo published v2.0.0 and v3.0.0 before
// anything checked.
func TestMajorVersionMustFitTheModulePath(t *testing.T) {
	for _, tag := range []string{"v2.0.0", "v3.1.4", "v10.0.0"} {
		err := majorFits(tag, "github.com/joeblew999/dev")
		if err == nil {
			t.Errorf("%s was allowed; this module path may only carry v0 and v1", tag)
			continue
		}
		if !strings.Contains(err.Error(), "may only carry v0 and v1") {
			t.Errorf("%s: the error should say why: %v", tag, err)
		}
	}
	// And a path that does say so carries its own major fine.
	if err := majorFits("v3.0.0", "github.com/joeblew999/dev/v3"); err != nil {
		t.Errorf("a /v3 path should carry v3: %v", err)
	}
	for _, tag := range []string{"v0.5.1", "v1.3.0", "v1.99.0"} {
		if err := majorFits(tag, "github.com/joeblew999/dev"); err != nil {
			t.Errorf("%s should be allowed: %v", tag, err)
		}
	}
}
