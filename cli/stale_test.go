package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// repoWithBinary makes a repo root holding a binary, and returns both. The
// binary stands in for os.Executable, which the test cannot move, so these
// tests exercise staleBuild's parts rather than staleBuild itself; the
// integration is covered by dev's own build.
func repoWithBinary(t *testing.T) (root, exe string) {
	t.Helper()
	root = t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "mise.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	exe = filepath.Join(root, ".bin", "cmd")
	if err := os.WriteFile(exe, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, exe
}

// A binary inside the repo is one this repo built, so the question applies.
// A release installed elsewhere carries prose fixed at its pinned version,
// which nothing in a consumer's repo can make stale.
func TestUnderTellsRepoBuildFromRelease(t *testing.T) {
	root, exe := repoWithBinary(t)
	if !under(root, exe) {
		t.Errorf("a binary in the repo's .bin should count as built here")
	}
	if under(root, filepath.Join(t.TempDir(), "dev")) {
		t.Errorf("a binary outside the repo should not")
	}
}

// The guard's whole point: a source edited after the build is found, so the
// command refuses instead of answering from bytes it knows are old.
func TestNewerSourceIsFound(t *testing.T) {
	root, exe := repoWithBinary(t)
	built := fileTime(t, exe)
	src := filepath.Join(root, "internal", "x")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	usage := filepath.Join(src, "usage.md")
	if err := os.WriteFile(usage, []byte("### x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(usage, built.Add(time.Minute), built.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := newestAfter(root, built); got != usage {
		t.Errorf("newer source: got %q, want %q", got, usage)
	}
}

// Output and documents are not build inputs. Treating them as sources would
// refuse every run after a manual is written — including the one that just
// wrote it.
func TestOutputAndDocumentsAreNotSources(t *testing.T) {
	root, exe := repoWithBinary(t)
	built := fileTime(t, exe)
	for _, dir := range []string{ShippedDir, ".claude", ".agents", ".plans", ".dist"} {
		d := filepath.Join(root, dir, "sub")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(d, "SKILL.md")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, built.Add(time.Minute), built.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if got := newestAfter(root, built); got != "" {
		t.Errorf("%s was treated as a source; it is output or a document", got)
	}
}

func fileTime(t *testing.T, p string) time.Time {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.ModTime()
}
