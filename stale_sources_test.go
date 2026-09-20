package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
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
				// `all:` asks go:embed to include dotfiles and is not part
				// of the path. Read as one, it named site/fly/all:public and
				// asked for a build source that could never match.
				name = strings.TrimPrefix(name, "all:")
				dir := filepath.Dir(p)
				// site/ is its own binary — the generator and the server that
				// carries the pages into a Fly image. This test is about
				// .bin/dev going stale while mise reports it fresh, and what
				// site/ embeds is compiled into neither .bin/dev nor the
				// manual it renders. Its output is generated and gitignored,
				// so listing it as a build source would make the build depend
				// on something the build writes.
				if strings.HasPrefix(filepath.ToSlash(dir), "site/") {
					continue
				}
				out = append(out, filepath.Join(dir, name))
			}
		}
	}
	return out
}

// buildSources is the sources list of mise.toml's build task, by name.
//
// It was three string cuts — find "sources = [", take what is before the
// next "]" — and the comment defending that said parsing the whole file
// would read every task. It read the wrong one instead. mise.toml has three
// sources lists now, and strings.Cut takes the first, so this worked only
// because [tasks.build] happens to be declared before [tasks."site:build"].
//
// Moving those two blocks past each other — a pure reordering, no change in
// meaning — made this read site:build's three globs and report nine embedded
// files as unguarded. The dangerous direction is the other one: land on a
// task with a broader list and the test passes while guarding nothing, which
// is exactly what it exists to prevent.
//
// BurntSushi/toml is already a direct dependency of this module. Asking it
// for one task by name is six lines and cannot read a different one.
func buildSources(t *testing.T) []string {
	t.Helper()
	var file struct {
		Tasks map[string]struct {
			Sources []string `toml:"sources"`
		} `toml:"tasks"`
	}
	if _, err := toml.DecodeFile("mise.toml", &file); err != nil {
		t.Fatal(err)
	}
	got := file.Tasks["build"].Sources
	if len(got) == 0 {
		t.Fatal("mise.toml's build task has no sources list")
	}
	return got
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
