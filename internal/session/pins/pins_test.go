// What a repo declared, held to what a pin has to be.
//
// Nothing here touches the network or a skills directory: these tests write a
// session.toml and read it back, which is the whole of what this package can
// be wrong about.
package pins

import (
	"os"
	"strings"
	"testing"
)

func writeFile(name, content string) error {
	return os.WriteFile(name, []byte(content), 0o644)
}

func TestLoadPins(t *testing.T) {
	t.Chdir(t.TempDir())
	content := "[source.a]\nrepo = \"org/a\"\nref = \"dddddddddddddddddddddddddddddddddddddddd\"\nskills = [\"x\", \"y\"]\n\n[source.b]\nrepo = \"org/repo\"\nref = \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"\nskills = [\"z\"]\n"
	if err := writeFile("session.toml", content); err != nil {
		t.Fatal(err)
	}
	p, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Names(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("names = %v", got)
	}
	a := p.Source["a"]
	if a.Repo != "org/a" || a.Ref != "dddddddddddddddddddddddddddddddddddddddd" || len(a.Skills) != 2 {
		t.Errorf("source a = %+v", a)
	}
	b := p.Source["b"]
	if b.Repo != "org/repo" || b.Ref != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || len(b.Skills) != 1 {
		t.Errorf("source b = %+v", b)
	}
}

func TestLoadPinsRejectsUnknownKeys(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := writeFile("session.toml", "[source.a]\nrepo = \"x/y\"\nref = \"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\"\nskills = [\"z\"]\nbogus = 1\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Error("loadPins succeeded with an unknown key, want an error naming the fix")
	}
}

func TestLoadPinsRejectsBadSource(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := writeFile("session.toml", "[source.a]\nskills = [\"z\"]\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Error("loadPins succeeded with no repo or ref, want an error naming the fix")
	}
}

// Another repo runs this through its own task runner, so the fix an error
// names has to come from that repo, not from this one's habits.
func TestSyncCommandComesFromPins(t *testing.T) {
	t.Chdir(t.TempDir())
	if got := readSyncCommand(); got != "dev session sync" {
		t.Errorf("with no %s at all, the advice is %q; want the binary", File, got)
	}
	if err := writeFile(File, "sync_command = \"just skills\"\n[source.a]\nrepo = \"o/r\"\nref = \""+strings.Repeat("b", 40)+"\"\nskills = [\"z\"]\n"); err != nil {
		t.Fatal(err)
	}
	if got := readSyncCommand(); got != "just skills" {
		t.Errorf("readSyncCommand() = %q; want the one %s names", got, File)
	}
}

// A repo spells verify the way it spells sync, so advice never tells a reader
// to run a command their repo does not have.
func TestVerifyIsSpelledLikeSync(t *testing.T) {
	for cmd, want := range map[string]string{
		"mise run session:sync": "mise run session:verify",
		"just skills":           "just skills verify",
		"dev session sync":      "dev session verify",
	} {
		if got := swapVerb(cmd, "sync", "verify"); got != want {
			t.Errorf("swapVerb(%q) = %q; want %q", cmd, got, want)
		}
	}
}

// A pin has to pin. `ref = "HEAD"` or a branch name reads as a version and is
// not one: upstream moves it, two clones of this repo get different skills
// from the same committed file, and nothing here would ever say so. It used to
// be accepted and then panic much later, slicing four characters as twelve.
func TestARefThatIsNotACommitIsRefused(t *testing.T) {
	for _, ref := range []string{"HEAD", "main", "v1.2.0", "c55ee46", strings.Repeat("g", 40)} {
		t.Run(ref, func(t *testing.T) {
			t.Chdir(t.TempDir())
			pin := "[source.a]\nrepo = \"o/r\"\nref = \"" + ref + "\"\nskills = [\"z\"]\n"
			if err := writeFile("session.toml", pin); err != nil {
				t.Fatal(err)
			}
			_, err := Load()
			if err == nil {
				t.Fatalf("ref %q was accepted, so this repo pins nothing", ref)
			}
			if !strings.Contains(err.Error(), ref) {
				t.Errorf("the error does not quote the ref it rejected: %v", err)
			}
		})
	}
	if !isCommit(strings.Repeat("c5", 20)) {
		t.Error("a real commit sha was called something else")
	}
}

// The repo is asked of GitHub as owner/name, and it also names the directory
// inside the tarball, so a half-written one has to be caught where it is read.
func TestARepoWithoutAnOwnerIsRefused(t *testing.T) {
	for _, repo := range []string{"skills", "github.com/o/r"} {
		t.Chdir(t.TempDir())
		if err := writeFile("session.toml", "[source.a]\nrepo = \""+repo+"\"\nref = \""+strings.Repeat("a", 40)+"\"\nskills = [\"z\"]\n"); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(); err == nil {
			t.Errorf("repo %q was accepted", repo)
		}
	}
}

func TestLoadPinsAllowsOnlyClaudeSettings(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := writeFile("session.toml", "[claude]\nblocked_plugins = [\"x@y\"]\n"); err != nil {
		t.Fatal(err)
	}
	p, err := Load()
	if err != nil {
		t.Fatalf("a repo pinning only its Claude settings was rejected: %v", err)
	}
	if len(p.Source) != 0 || len(p.Claude.BlockedPlugins) != 1 {
		t.Fatalf("got %+v", p)
	}
}

// A preset is compiled into dev, so nobody reviewing session.toml will ever
// see it. It is held to exactly the rule a repo's own source is held to —
// owner/name, a full commit, at least one skill — because a preset that fails
// it would break every repo that opted in, at their next sync, with a message
// blaming their file.
func TestEveryPresetDevShipsIsAValidPin(t *testing.T) {
	if len(Presets) == 0 {
		t.Fatal("dev ships no presets, so the one line a repo writes buys nothing")
	}
	for name, preset := range Presets {
		if preset.Why == "" {
			t.Errorf("preset %q does not say what it is for", name)
		}
		if len(preset.Source) == 0 {
			t.Errorf("preset %q draws from nowhere", name)
		}
		for source, src := range preset.Source {
			if err := src.check(source); err != nil {
				t.Errorf("preset %q, source %q: %v", name, source, err)
			}
		}
	}
}

// Two presets, or a preset and the repo's own source, cannot both claim a
// name: whichever won would give a repo a skill it did not ask for under a
// name it did.
func TestAPresetCannotTakeANameTheRepoUses(t *testing.T) {
	t.Chdir(t.TempDir())
	pin := "presets = [\"recommended\"]\n[source.mattpocock]\nrepo = \"me/mine\"\nref = \"" +
		strings.Repeat("a", 40) + "\"\nskills = [\"x\"]\n"
	if err := writeFile(File, pin); err != nil {
		t.Fatal(err)
	}
	_, err := Load()
	if err == nil {
		t.Fatal("a preset silently took over a source the repo declared")
	}
	if !strings.Contains(err.Error(), "mattpocock") {
		t.Errorf("the error does not name the clash: %v", err)
	}
}

// A preset that does not exist is a typo, and the message lists what does —
// a repo adopting this has no other way to find out.
func TestAnUnknownPresetListsTheRealOnes(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := writeFile(File, "presets = [\"reccomended\"]\n"); err != nil {
		t.Fatal(err)
	}
	_, err := Load()
	if err == nil {
		t.Fatal("an unknown preset was accepted")
	}
	for want := range Presets {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not name the %q preset that does exist: %v", want, err)
		}
	}
}

// A preset source is dev's, and bump rewrites session.toml, where it has no
// line. Declared is what keeps those apart.
func TestDeclaredIsOnlyWhatTheRepoWrote(t *testing.T) {
	t.Chdir(t.TempDir())
	pin := "presets = [\"recommended\"]\n[source.mine]\nrepo = \"me/mine\"\nref = \"" +
		strings.Repeat("a", 40) + "\"\nskills = [\"x\"]\n"
	if err := writeFile(File, pin); err != nil {
		t.Fatal(err)
	}
	p, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.Declared(), ","); got != "mine" {
		t.Errorf("Declared() = %q; want only the repo's own source", got)
	}
	if len(p.Names()) <= len(p.Declared()) {
		t.Error("the preset's sources did not arrive at all")
	}
	if preset, ok := p.Source["mattpocock"].FromPreset(); !ok || preset != "recommended" {
		t.Errorf("a preset source does not say which preset it came from: %q", preset)
	}
}
