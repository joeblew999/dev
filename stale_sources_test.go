package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/joeblew999/dev/cli"
)

// Every file go:embed compiles in has to be in mise.toml's build sources, or
// editing it leaves .bin/dev stale while mise reports the build fresh — and
// `dev skill` then rewrites every manual from the old bytes and says it
// succeeded. AGENTS.md warns about it; this is the warning made mechanical,
// because a warning nobody runs is a comment.
func TestEveryEmbeddedFileIsABuildSource(t *testing.T) {
	globs := buildSources(t)
	for _, f := range embedded(t) {
		if !matchesAny(globs, f) {
			t.Errorf("%s is compiled in by go:embed but no build source glob matches it; "+
				"add it to sources in mise.toml, or editing it leaves the binary stale "+
				"while mise calls it fresh", f)
		}
	}
}

// embedded is every path a //go:embed directive names, relative to the repo.
func embedded(t *testing.T) []string {
	t.Helper()
	directive := regexp.MustCompile(`(?m)^//go:embed\s+(.+)$`)
	var out []string
	for _, p := range goFiles(t) {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, m := range directive.FindAllStringSubmatch(string(data), -1) {
			for name := range strings.FieldsSeq(m[1]) {
				out = append(out, filepath.Join(filepath.Dir(p), name))
			}
		}
	}
	return out
}

// buildSources is the sources list of mise.toml's build task, read as the
// lines between its brackets. Parsing the whole file with a TOML decoder
// would read every task; this reads the one that matters.
func buildSources(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("mise.toml")
	if err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(string(data), "sources = [")
	if !ok {
		t.Fatal("mise.toml's build task has no sources list")
	}
	list, _, ok := strings.Cut(after, "]")
	if !ok {
		t.Fatal("mise.toml's sources list is not closed")
	}
	return cli.Collect(strings.Split(list, ","), func(s string) (string, bool) {
		s = strings.Trim(strings.TrimSpace(s), `"`)
		return s, s != "" && !strings.HasPrefix(s, "#")
	})
}

// matchesAny is mise's globbing as far as this needs it: ** crosses
// directories, * does not.
func matchesAny(globs []string, path string) bool {
	path = filepath.ToSlash(path)
	for _, g := range globs {
		if g == path {
			return true
		}
		if after, ok := strings.CutPrefix(g, "**/"); ok {
			if ok, _ := filepath.Match(after, filepath.Base(path)); ok {
				return true
			}
			continue
		}
		if ok, _ := filepath.Match(g, path); ok {
			return true
		}
	}
	return false
}
