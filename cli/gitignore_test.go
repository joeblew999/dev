package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIgnore(t *testing.T) {
	for _, tc := range []struct {
		name, start string
		want        []string // substrings the result must hold
		unchanged   bool
	}{
		{name: "missing file is created", start: "", want: []string{".bin\n", ".dist\n"}},
		{name: "already there in the same shape", start: ".bin\n.dist\n", unchanged: true},
		{name: "already there anchored", start: "/.bin/\n/.dist/\n", unchanged: true},
		{name: "an old repo gains both", start: "bin\n/dist/\n", want: []string{"bin\n", ".bin\n", ".dist\n"}},
		{name: "a comment naming it does not count", start: "# .bin\n", want: []string{".bin\n", ".dist\n"}},
		{name: "no trailing newline", start: "vendor", want: []string{"vendor\n", ".bin\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.start != "" {
				if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(tc.start), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := Ignore(dir, ".bin", ".dist"); err != nil {
				t.Fatal(err)
			}
			got := read(t, dir)
			if tc.unchanged {
				if got != tc.start {
					t.Fatalf("file was rewritten:\nwant %q\ngot  %q", tc.start, got)
				}
				return
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			// Twice is the same as once.
			if err := Ignore(dir, ".bin", ".dist"); err != nil {
				t.Fatal(err)
			}
			if again := read(t, dir); again != got {
				t.Errorf("not idempotent:\nfirst  %q\nsecond %q", got, again)
			}
		})
	}
}
