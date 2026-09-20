package session

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeblew999/dev/cli"
)

func writeFile(name, content string) error {
	return os.WriteFile(name, []byte(content), 0o644)
}

// ps prints start times in the machine's locale, which broke the shell version
// of this twice, so elapsed time is parsed here and tested.
func TestParseElapsed(t *testing.T) {
	cases := map[string]time.Duration{
		"05:09":       5*time.Minute + 9*time.Second,
		"17:32:20":    17*time.Hour + 32*time.Minute + 20*time.Second,
		"1-04:05:06":  24*time.Hour + 4*time.Hour + 5*time.Minute + 6*time.Second,
		"12-00:00:00": 12 * 24 * time.Hour,
		"  01:00  ":   time.Minute,
	}
	for etime, want := range cases {
		got, err := parseElapsed(etime)
		if err != nil || got != want {
			t.Errorf("parseElapsed(%q) = %v, %v; want %v", etime, got, err, want)
		}
	}
	for _, bad := range []string{"", "nonsense", "10", "a:b", "x-01:00"} {
		if _, err := parseElapsed(bad); err == nil {
			t.Errorf("parseElapsed(%q) succeeded, want an error naming the format", bad)
		}
	}
}

func TestIsClaudeBinary(t *testing.T) {
	yes := []string{
		"/Users/a/.vscode/extensions/anthropic.claude-code-2.1.271-darwin-arm64/resources/native-binary/claude",
		"/Users/a/.local/bin/claude",
	}
	no := []string{"claude", "/bin/zsh -c claude", "/usr/bin/claude-helper", "/opt/claude/other"}
	for _, command := range yes {
		if !isClaudeBinary(command) {
			t.Errorf("isClaudeBinary(%q) = false, want true", command)
		}
	}
	for _, command := range no {
		if isClaudeBinary(command) {
			t.Errorf("isClaudeBinary(%q) = true, want false", command)
		}
	}
}

func TestDiffFiles(t *testing.T) {
	have := skillFiles{"gsx/SKILL.md": []byte("a"), "old/SKILL.md": []byte("x")}
	want := skillFiles{"gsx/SKILL.md": []byte("b"), "new/SKILL.md": []byte("c")}
	got := strings.Join(diffFiles(have, want), "; ")
	if got != "changed: gsx/SKILL.md; missing: new/SKILL.md; unexpected: old/SKILL.md" {
		t.Errorf("diffFiles() = %q", got)
	}
	if !sameFiles(want, want) {
		t.Error("sameFiles() = false for identical sets")
	}
}

func TestLoadPins(t *testing.T) {
	t.Chdir(t.TempDir())
	content := "[source.a]\nrepo = \"org/a\"\nref = \"dddddddddddddddddddddddddddddddddddddddd\"\nskills = [\"x\", \"y\"]\n\n[source.b]\nrepo = \"org/repo\"\nref = \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"\nskills = [\"z\"]\n"
	if err := writeFile("session.toml", content); err != nil {
		t.Fatal(err)
	}
	p, err := loadPins()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.names(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
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
	if _, err := loadPins(); err == nil {
		t.Error("loadPins succeeded with an unknown key, want an error naming the fix")
	}
}

func TestLoadPinsAllowsOnlyClaudeSettings(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := writeFile("session.toml", "[claude]\nblocked_plugins = [\"x@y\"]\n"); err != nil {
		t.Fatal(err)
	}
	p, err := loadPins()
	if err != nil {
		t.Fatalf("a repo pinning only its Claude settings was rejected: %v", err)
	}
	if len(p.Source) != 0 || len(p.Claude.BlockedPlugins) != 1 {
		t.Fatalf("got %+v", p)
	}
}

func TestLoadPinsRejectsBadSource(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := writeFile("session.toml", "[source.a]\nskills = [\"z\"]\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPins(); err == nil {
		t.Error("loadPins succeeded with no repo or ref, want an error naming the fix")
	}
}

func TestWarnStaleSessionsSaysNothingWithoutSkills(t *testing.T) {
	var out strings.Builder
	t.Chdir(t.TempDir())
	warnStaleSessions(&out, time.Now())
	if out.Len() != 0 {
		t.Errorf("warned without a skills directory: %q", out.String())
	}
}
func TestSyncSettingsKeepsWhatItDoesNotOwn(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(settingsFile, `{"hooks":{"Stop":[]},"enabledPlugins":{"stale@old":false}}`); err != nil {
		t.Fatal(err)
	}
	c := claudePins{BlockedPlugins: []string{"a@b"}, ApproveMCPServers: true}
	if err := syncSettings(io.Discard, c); err != nil {
		t.Fatal(err)
	}
	have, err := readSettings()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := have["hooks"]; !ok {
		t.Error("sync dropped hooks, which it does not own")
	}
	// What sync just wrote is what check accepts; a finding here would mean
	// the two disagree about what [claude] implies.
	if found, _, err := settingsFindings(c); err != nil || len(found) > 0 {
		t.Errorf("check found %v (%v) right after sync", found, err)
	}
	blocked, _ := have["enabledPlugins"].(map[string]any)
	if blocked["a@b"] != false || len(blocked) != 1 {
		t.Errorf("enabledPlugins = %v; want exactly the blocked list", blocked)
	}
}

func TestCheckSettingsCatchesHandEdits(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(settingsFile, `{"enabledPlugins":{"a@b":true}}`); err != nil {
		t.Fatal(err)
	}
	// The pins the finding is checked against come from session.toml, which
	// this test wrote above.
	found, _, err := settingsFindings(claudePins{BlockedPlugins: []string{"a@b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 {
		t.Fatal("settings that do not match the pins passed the check")
	}
	// Every finding names the fix, which is what makes a report actionable.
	for _, f := range found {
		if !strings.Contains(f.Fix, syncCmd) {
			t.Errorf("%s does not name %q as the fix: %q", f.ID, syncCmd, f.Fix)
		}
	}
}

// Another repo runs this through its own task runner, so the fix an error
// names has to come from that repo, not from this one's habits.
func TestSyncCommandComesFromPins(t *testing.T) {
	t.Chdir(t.TempDir())
	defer func(old string) { syncCmd = old }(syncCmd)
	syncCmd = "dev session sync"
	if err := writeFile("session.toml", "sync_command = \"just skills\"\n[source.a]\nrepo = \"o/r\"\nref = \"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\"\nskills = [\"z\"]\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPins(); err != nil {
		t.Fatal(err)
	}
	if syncCmd != "just skills" {
		t.Errorf("syncCmd = %q; want the one session.toml names", syncCmd)
	}
}

// A repo that never mentions connectors must not have them turned off behind
// its back, so an absent key writes no setting at all.
func TestConnectorsUnmanagedWhenUnset(t *testing.T) {
	if _, ok := wantSettings(claudePins{})["disableClaudeAiConnectors"]; ok {
		t.Error("an unset claude_ai_connectors disabled them anyway")
	}
	off := false
	if want := wantSettings(claudePins{ClaudeAIConnectors: &off}); want["disableClaudeAiConnectors"] != true {
		t.Errorf("claude_ai_connectors = false did not disable them: %v", want)
	}
	on := true
	if _, ok := wantSettings(claudePins{ClaudeAIConnectors: &on})["disableClaudeAiConnectors"]; ok {
		t.Error("claude_ai_connectors = true wrote a setting; it cannot force them on")
	}
}
func TestCheckPortablePathsCatchesAbsoluteCommands(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(".mcp.json", `{"mcpServers":{"hk":{"command":"/opt/homebrew/bin/mise"}}}`); err != nil {
		t.Fatal(err)
	}
	found, _, err := portableFindings()
	if err != nil {
		t.Fatal(err)
	}
	said := strings.Join(cli.Map(found, func(f cli.Finding) string { return f.Message }), " ")
	if !strings.Contains(said, "/opt/homebrew/bin/mise") {
		t.Errorf("the offending command was not named: %q", said)
	}
	// Nested inside a hooks array, which is where the other one hid.
	if err := writeFile(settingsFile, `{"hooks":{"Stop":[{"hooks":[{"command":"/usr/local/bin/x"}]}]}}`); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(".mcp.json", `{"mcpServers":{"hk":{"command":"mise"}}}`); err != nil {
		t.Fatal(err)
	}
	nested, _, err := portableFindings()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(cli.Map(nested, func(f cli.Finding) string { return f.Message }), " "), "/usr/local/bin/x") {
		t.Errorf("a command nested in hooks was missed: %v", nested)
	}
	if err := writeFile(settingsFile, `{"hooks":{"Stop":[{"hooks":[{"command":"mise exec -- hk"}]}]}}`); err != nil {
		t.Fatal(err)
	}
	if found, _, err := portableFindings(); err != nil || len(found) > 0 {
		t.Errorf("bare commands should pass: %v (%v)", found, err)
	}
}

func TestSessionLockRecordsWhichClaudeCodeWroteIt(t *testing.T) {
	names, by := parseSessionLock(formatSessionLock([]string{"gsx", "wrangler"}, "2.1.272"))
	if by != "2.1.272" || strings.Join(names, ",") != "gsx,wrangler" {
		t.Fatalf("got %v by %q", names, by)
	}
	names, by = parseSessionLock("# an older lock\nwrangler\ngsx\n")
	if by != "" || strings.Join(names, ",") != "gsx,wrangler" {
		t.Fatalf("old format: got %v by %q", names, by)
	}
}

// A bare --update once parsed as --update= and so as false: verify refused
// the very change it was asked to record. The flag is registered with cli
// now, so this drives the real registration rather than a reader beside it —
// including the `--update=` a mise task sends when its variable is unset, and
// the stray positional cli rejects for a verb that declares no Args.
func TestVerifyUpdateFlag(t *testing.T) {
	for _, c := range []struct {
		args       []string
		update, ok bool
	}{
		{nil, false, true},
		{[]string{"--update"}, true, true},
		{[]string{"--update=true"}, true, true},
		{[]string{"--update=false"}, false, true},
		{[]string{"--update="}, false, true},
		{[]string{"--other"}, false, false},
		{[]string{"--update", "x"}, false, false},
	} {
		fs := cli.Flags("session verify", io.Discard)
		VerifyFlags(fs)
		rest, err := cli.ParseInterleaved(fs, c.args)
		if err == nil && len(rest) > 0 {
			err = cli.Usagef("session verify: takes no arguments")
		}
		if ok := err == nil; ok != c.ok {
			t.Errorf("parse(%q) error %v; want ok %v", c.args, err, c.ok)
			continue
		}
		if err != nil {
			continue
		}
		if got := cli.Given(fs, "update"); got != c.update {
			t.Errorf("parse(%q) update %v; want %v", c.args, got, c.update)
		}
	}
}

// A repo that vendors no skills, has an empty lock,
// and that is not an error: there is nothing to check.
func TestEmptyLockIsNoSkills(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(skillsDir, lockFile), ""); err != nil {
		t.Fatal(err)
	}
	files, err := lockedFiles()
	if err != nil || len(files) != 0 {
		t.Fatalf("lockedFiles() = %v, %v; want none and no error", files, err)
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
			_, err := loadPins()
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
		if _, err := loadPins(); err == nil {
			t.Errorf("repo %q was accepted", repo)
		}
	}
}
