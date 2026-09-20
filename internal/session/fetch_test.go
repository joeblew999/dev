// Taking skills out of an upstream's tarball.
//
// Both of these were wrong for as long as the code existed and neither could
// show itself: the prefix was written for the one upstream in use, whose
// repository happens to be named `skills`, and every other one failed as
// "skill not found" — a sentence that sends a reader to check the name, which
// was never the problem.
package session

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/joeblew999/dev/cli"
)

// tarball is an upstream as codeload serves it: one top directory named after
// the repository, with everything below it.
func tarball(t *testing.T, top string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	w := tar.NewWriter(gz)
	for name, body := range files {
		if err := w.WriteHeader(&tar.Header{Name: top + "/" + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const ref = "c55ee46073ed923f86ce59a5eb3b6d895095d1b7"

// The tarball's top directory is named after the repository, so the prefix has
// to be too. Hardcoding `skills` worked only for a repository called that.
func TestPrefixIsNamedAfterTheRepositoryNotTheConvention(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  sourcePins
		want string
	}{
		{"a repo that is called skills", sourcePins{Repo: "cloudflare/skills", Ref: ref}, "skills-" + ref + "/skills/"},
		{"a repo that is not", sourcePins{Repo: "obra/superpowers", Ref: ref}, "superpowers-" + ref + "/skills/"},
		{"skills under some other directory", sourcePins{Repo: "acme/tools", Ref: ref, Dir: "agent/skills"}, "tools-" + ref + "/agent/skills/"},
		{"skills at the repository root", sourcePins{Repo: "acme/tools", Ref: ref, Dir: "/"}, "tools-" + ref + "/"},
	} {
		if got := tc.src.prefix(); got != tc.want {
			t.Errorf("%s: prefix = %q; want %q", tc.name, got, tc.want)
		}
	}
}

// Upstreams file skills by category; an agent reads one level below
// .claude/skills and no deeper. So a pinned `engineering/tdd` is fetched from
// there and lands as `tdd`, or it is vendored somewhere nothing looks.
func TestANestedSkillIsVendoredFlat(t *testing.T) {
	archive := tarball(t, "skills-"+ref, map[string]string{
		"skills/engineering/tdd/SKILL.md":        "# tdd",
		"skills/engineering/tdd/agents/notes.md": "notes",
		"skills/engineering/triage/SKILL.md":     "# triage",
		"skills/productivity/handoff/SKILL.md":   "# handoff",
	})
	src := sourcePins{Repo: "mattpocock/skills", Ref: ref}
	files := skillFiles{}
	name := "engineering/tdd"
	found, err := copyTar(files, archive, src.prefix()+name+"/", vendored(name))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("found nothing, so a nested skill cannot be pinned at all")
	}
	got := cli.SortedKeys(files)
	want := []string{"tdd/SKILL.md", "tdd/agents/notes.md"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("vendored %v; want %v — a skill one level deeper is one an agent never loads", got, want)
	}
}

// A prefix that matches nothing is not an error here: only the caller knows
// which pin asked for it, and its message is the one that names the file to
// fix. copyTar saying so itself is how that sentence got written twice.
func TestAMissMatchesNothingRatherThanFailing(t *testing.T) {
	archive := tarball(t, "skills-"+ref, map[string]string{"skills/tdd/SKILL.md": "# tdd"})
	files := skillFiles{}
	found, err := copyTar(files, archive, "skills-"+ref+"/skills/nope/", "nope")
	if err != nil {
		t.Fatalf("a missing skill is not a broken archive: %v", err)
	}
	if found || len(files) != 0 {
		t.Errorf("found = %v with %d files; want nothing", found, len(files))
	}
}

func TestVendoredTakesTheLastElement(t *testing.T) {
	for in, want := range map[string]string{
		"tdd":               "tdd",
		"engineering/tdd":   "tdd",
		"/engineering/tdd/": "tdd",
		"a/b/c/deep-skill":  "deep-skill",
	} {
		if got := vendored(in); got != want {
			t.Errorf("vendored(%q) = %q; want %q", in, got, want)
		}
	}
}
