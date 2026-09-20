// What a repository holds at one commit.
//
// The archives here are built in memory, shaped like the ones codeload really
// serves — one top directory named after the repository — because that name
// was hardcoded as `skills` for as long as the code existed and could not show
// itself: the one upstream in use happens to be called that, and every other
// one failed as "skill not found", a sentence that sends a reader to check the
// name, which was never the problem.
package upstream

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

const ref = "c55ee46073ed923f86ce59a5eb3b6d895095d1b7"

// archive is an upstream as codeload serves it.
func archive(t *testing.T, repo string, files map[string]string) *Archive {
	t.Helper()
	top := repo[strings.LastIndex(repo, "/")+1:] + "-" + ref + "/"
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	w := tar.NewWriter(gz)
	for name, body := range files {
		if err := w.WriteHeader(&tar.Header{Name: top + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	for _, closer := range []func() error{w.Close, gz.Close} {
		if err := closer(); err != nil {
			t.Fatal(err)
		}
	}
	got, err := untar(buf.Bytes(), top)
	if err != nil {
		t.Fatal(err)
	}
	return &Archive{Repo: repo, Ref: ref, files: got}
}

// The top directory is named after the repository, so stripping it has to be
// too. A repository not called `skills` was unusable.
func TestTheTopDirectoryIsStrippedWhateverTheRepositoryIsCalled(t *testing.T) {
	for _, repo := range []string{"cloudflare/skills", "obra/superpowers", "acme/some-tools"} {
		a := archive(t, repo, map[string]string{"skills/tdd/SKILL.md": "# tdd"})
		if !a.Has("skills/tdd") {
			t.Errorf("%s: the archive's own skills/tdd was not found", repo)
		}
	}
}

// Upstreams file skills by category; an agent reads one level below
// .claude/skills and no deeper. So Dir takes from one path and files under
// another, and the caller decides what that other one is.
func TestDirFilesUnderTheNameItIsGiven(t *testing.T) {
	a := archive(t, "mattpocock/skills", map[string]string{
		"skills/engineering/tdd/SKILL.md":        "# tdd",
		"skills/engineering/tdd/agents/notes.md": "notes",
		"skills/engineering/triage/SKILL.md":     "# triage",
	})
	out := map[string][]byte{}
	if !a.Dir(out, "skills/engineering/tdd", "tdd") {
		t.Fatal("found nothing, so a nested skill cannot be pinned at all")
	}
	want := map[string]string{"tdd/SKILL.md": "# tdd", "tdd/agents/notes.md": "notes"}
	if len(out) != len(want) {
		t.Fatalf("took %d files; want %d", len(out), len(want))
	}
	for name, body := range want {
		if string(out[name]) != body {
			t.Errorf("%s = %q; want %q", name, out[name], body)
		}
	}
	// A sibling under the same parent is not swept in by a prefix match.
	if _, leaked := out["tdd/../triage/SKILL.md"]; leaked {
		t.Error("a sibling skill came along")
	}
}

// A path that matches nothing is not an error: only the caller knows which pin
// asked for it, and its message is the one that names the file to fix.
func TestAMissMatchesNothingRatherThanFailing(t *testing.T) {
	a := archive(t, "obra/superpowers", map[string]string{"skills/tdd/SKILL.md": "# tdd"})
	out := map[string][]byte{}
	if a.Dir(out, "skills/nope", "nope") || len(out) != 0 {
		t.Errorf("took %d files for a path that is not there", len(out))
	}
	// And a prefix that is a partial name does not match a longer one.
	if a.Dir(out, "skills/td", "td") {
		t.Error("skills/td matched skills/tdd")
	}
}

// An upstream that ships as a Claude Code plugin declares its own skills, and
// that list is the authority: it names where each one is filed, and leaves out
// the ones the repository keeps but does not ship.
func TestAPluginDeclaresItsOwnSkills(t *testing.T) {
	a := archive(t, "mattpocock/skills", map[string]string{
		PluginManifest: `{"name":"mattpocock-skills","version":"1.2.3",
			"skills":["./skills/engineering/tdd","./skills/productivity/handoff"]}`,
		"skills/engineering/tdd/SKILL.md":    "# tdd",
		"skills/deprecated/old-one/SKILL.md": "# not shipped",
	})
	plugin, isPlugin, err := a.Plugin()
	if err != nil {
		t.Fatal(err)
	}
	if !isPlugin {
		t.Fatal("a repository with a plugin manifest was not read as one")
	}
	if plugin.Name != "mattpocock-skills" || len(plugin.Skills) != 2 {
		t.Fatalf("plugin = %+v", plugin)
	}
	if strings.Contains(strings.Join(plugin.Skills, " "), "deprecated") {
		t.Error("the manifest was read as listing a skill it does not ship")
	}
}

// Plenty of upstreams are a skills directory and nothing more, and one that
// ships a manifest may still not list its skills in it — superpowers does
// exactly that. Neither is an error, which is why convention is tried first.
func TestNotAPluginIsNotAnError(t *testing.T) {
	a := archive(t, "obra/superpowers", map[string]string{"skills/tdd/SKILL.md": "# tdd"})
	if _, isPlugin, err := a.Plugin(); err != nil || isPlugin {
		t.Errorf("Plugin() = %v, %v; want no manifest and no error", isPlugin, err)
	}
	b := archive(t, "obra/superpowers", map[string]string{
		PluginManifest:        `{"name":"superpowers","version":"6.4.1"}`,
		"skills/tdd/SKILL.md": "# tdd",
	})
	plugin, isPlugin, err := b.Plugin()
	if err != nil || !isPlugin {
		t.Fatalf("Plugin() = %v, %v", isPlugin, err)
	}
	if len(plugin.Skills) != 0 {
		t.Errorf("skills = %v; want none, so convention decides", plugin.Skills)
	}
}

// A manifest that is not the JSON it should be names the repository, because
// the reader's next question is whose file it was.
func TestBadManifestNamesTheRepository(t *testing.T) {
	a := archive(t, "acme/tools", map[string]string{PluginManifest: "{oh no"})
	_, isPlugin, err := a.Plugin()
	if err == nil {
		t.Fatal("broken JSON was accepted")
	}
	if !isPlugin || !strings.Contains(err.Error(), "acme/tools") {
		t.Errorf("error does not name the repository: %v", err)
	}
}

// A skill is a directory with a SKILL.md in it. Deriving that from the files
// is what makes an upstream usable whatever it calls its folders, and whether
// or not it says anything about itself — superpowers ships a plugin manifest
// that lists no skills at all, so nothing it declares could resolve a name.
func TestSkillsAreFoundFromTheFiles(t *testing.T) {
	a := archive(t, "obra/superpowers", map[string]string{
		PluginManifest:                         `{"name":"superpowers"}`,
		"skills/executing-plans/SKILL.md":      "# plans",
		"skills/systematic-debugging/SKILL.md": "# debugging",
		"skills/systematic-debugging/extra.md": "more",
		"docs/README.md":                       "not a skill",
		"tests/fixtures/thing/notes.md":        "nor this",
	})
	got := strings.Join(a.Skills(), " ")
	want := "skills/executing-plans skills/systematic-debugging"
	if got != want {
		t.Errorf("Skills() = %q; want %q", got, want)
	}
}

// Find searches what the repository declares before what its files show, and
// takes a name that is already a path at its word.
func TestFindSearchesDeclaredThenDerived(t *testing.T) {
	a := archive(t, "mattpocock/skills", map[string]string{
		PluginManifest:                         `{"name":"m","skills":["./skills/engineering/tdd"]}`,
		"skills/engineering/tdd/SKILL.md":      "# tdd",
		"skills/productivity/handoff/SKILL.md": "# handoff",
	})
	for _, tc := range []struct{ ask, want string }{
		{"tdd", "skills/engineering/tdd"},                    // from the manifest
		{"handoff", "skills/productivity/handoff"},           // from the files
		{"skills/engineering/tdd", "skills/engineering/tdd"}, // already a path
	} {
		got, ok := a.Find(tc.ask)
		if !ok || got != tc.want {
			t.Errorf("Find(%q) = %q, %v; want %q", tc.ask, got, ok, tc.want)
		}
	}
	if at, ok := a.Find("not-a-skill"); ok {
		t.Errorf("Find found %q for a name nothing has", at)
	}
}
