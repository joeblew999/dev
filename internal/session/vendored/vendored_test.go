// The skill set on disk.
//
// Nothing here knows where a skill came from, so these tests write files and
// read them back — which is the whole of what this package can be wrong about.
package vendored

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ourOwn puts a skill this repo wrote into every agent's directory — the
// thing sync must leave alone, since `dev skill` writes one per command and
// the lock has never heard of it.
func ourOwn(t *testing.T, names ...string) {
	t.Helper()
	for _, dir := range Dirs() {
		for _, name := range names {
			if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := writeFile(filepath.Join(dir, name, "SKILL.md"), "# "+name); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func writeFile(name, content string) error {
	return os.WriteFile(name, []byte(content), 0o644)
}

func TestDiffFiles(t *testing.T) {
	have := Set{"gsx/SKILL.md": []byte("a"), "old/SKILL.md": []byte("x")}
	want := Set{"gsx/SKILL.md": []byte("b"), "new/SKILL.md": []byte("c")}
	got := strings.Join(diffFiles(have, want), "; ")
	if got != "changed: gsx/SKILL.md; missing: new/SKILL.md; unexpected: old/SKILL.md" {
		t.Errorf("diffFiles() = %q", got)
	}
	if !Same(want, want) {
		t.Error("Same() = false for identical sets")
	}
}

// A repo that vendors no skills, has an empty lock,
// and that is not an error: there is nothing to check.
func TestEmptyLockIsNoSkills(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(Primary(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(Primary(), LockFile), ""); err != nil {
		t.Fatal(err)
	}
	files, err := Locked()
	if err != nil || len(files) != 0 {
		t.Fatalf("Locked() = %v, %v; want none and no error", files, err)
	}
}

// One set, every destination. An agent reads its own directory and no other,
// so a skill written to one alone is a skill the rest cannot see — the exact
// half that vendored skills used to be missing, while `dev skills` reported it
// and nothing could act on it.
func TestWriteReachesEveryAgentsDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	set := Set{
		"tdd/SKILL.md": []byte("# tdd"),
		LockFile:       []byte("tdd\tgithub.com/x/y@abc123abc123\n"),
	}
	if err := Write(set); err != nil {
		t.Fatal(err)
	}
	for _, dir := range Dirs() {
		have, err := Read(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !Same(have, set) {
			t.Errorf("%s holds %v; want the same set as everywhere else", dir, diffFiles(have, set))
		}
	}
	if len(Dirs()) < 2 {
		t.Fatal("there is only one destination, so this test proves nothing")
	}
}

// A sync that drops a skill removes it from every destination, and leaves
// alone anything it never owned.
func TestWriteRemovesOnlyWhatItOwned(t *testing.T) {
	t.Chdir(t.TempDir())
	first := Set{
		"tdd/SKILL.md":  []byte("# tdd"),
		"gone/SKILL.md": []byte("# gone"),
		LockFile:        []byte("tdd\tx\ngone\tx\n"),
	}
	if err := Write(first); err != nil {
		t.Fatal(err)
	}
	// A skill the repo wrote itself, which no sync owns.
	ourOwn(t, "ours")
	second := Set{"tdd/SKILL.md": []byte("# tdd"), LockFile: []byte("tdd\tx\n")}
	if err := Write(second); err != nil {
		t.Fatal(err)
	}
	for _, dir := range Dirs() {
		have, err := Read(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, still := have["gone/SKILL.md"]; still {
			t.Errorf("%s still holds a skill the pins dropped", dir)
		}
		if _, kept := have["ours/SKILL.md"]; !kept {
			t.Errorf("%s lost a skill the repo wrote itself, which sync does not own", dir)
		}
	}
}

// Undo takes back what sync owns and nothing else. The skills a repo writes
// itself sit in the same directory — `dev skill` writes a manual for each of a
// command's own verbs — and they are not sync's to remove. The lock is the
// list of what is, and it is the only thing consulted.
func TestRemoveTakesBackOnlyWhatSyncOwns(t *testing.T) {
	t.Chdir(t.TempDir())
	set := Set{
		"tdd/SKILL.md":           []byte("# tdd"),
		"brainstorming/SKILL.md": []byte("# brainstorming"),
		LockFile:                 []byte("brainstorming\tgithub.com/obra/superpowers@abc123abc123\ntdd\tgithub.com/mattpocock/skills@abc123abc123\n"),
	}
	if err := Write(set); err != nil {
		t.Fatal(err)
	}
	// The repo's own, written by `dev skill`, in the very same directory.
	ourOwn(t, "dev", "cli")

	went, err := Remove()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(went, ",") != "brainstorming,tdd" {
		t.Errorf("Remove() reported %v; want the two it vendored", went)
	}
	for _, dir := range Dirs() {
		have, err := Read(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, gone := range []string{"tdd/SKILL.md", "brainstorming/SKILL.md", LockFile} {
			if _, still := have[gone]; still {
				t.Errorf("%s still holds %s, which sync owned", dir, gone)
			}
		}
		for _, kept := range []string{"dev/SKILL.md", "cli/SKILL.md"} {
			if _, ok := have[kept]; !ok {
				t.Errorf("%s lost %s, which this repo wrote and sync never owned", dir, kept)
			}
		}
	}
}

// Undo on a repo that vendored nothing is not an error and does nothing.
func TestRemoveWithNothingVendored(t *testing.T) {
	t.Chdir(t.TempDir())
	went, err := Remove()
	if err != nil {
		t.Fatalf("Remove() on a repo with no lock: %v", err)
	}
	if len(went) != 0 {
		t.Errorf("Remove() reported %v; want nothing", went)
	}
}

// check and sync have to agree about what is sync's. They did not: sync leaves
// a skill the repo wrote itself exactly where it is, and check called that
// same file unexpected drift and said to sync it away — which sync would then
// decline to do. A repo with both its own skills and vendored ones failed
// forever, which is the normal repo on this stack, and adopting a preset here
// found it in under a minute.
func TestCheckIgnoresWhatSyncDoesNotOwn(t *testing.T) {
	t.Chdir(t.TempDir())
	set := Set{
		"tdd/SKILL.md": []byte("# tdd"),
		LockFile:       []byte("tdd\tgithub.com/mattpocock/skills@abc123abc123\ntdd/SKILL.md\t" + Hash([]byte("# tdd")) + "\n"),
	}
	if err := Write(set); err != nil {
		t.Fatal(err)
	}
	// The repo's own, written by `dev skill`, beside the vendored one.
	ourOwn(t, "dev")
	want, err := Locked()
	if err != nil {
		t.Fatal(err)
	}
	diff, _, err := Diff(want)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) > 0 {
		t.Errorf("check reported %v; a skill sync never owned is not drift", diff)
	}
	// A vendored file that really did change is still caught.
	if err := writeFile(filepath.Join(Primary(), "tdd", "SKILL.md"), "tampered"); err != nil {
		t.Fatal(err)
	}
	diff, _, err = Diff(want)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) != 1 || !strings.Contains(diff[0], "tdd/SKILL.md") {
		t.Errorf("check reported %v; want the one vendored file that changed", diff)
	}
}
