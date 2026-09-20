package session

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/session/pins"
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
	c := pins.Claude{BlockedPlugins: []string{"a@b"}, ApproveMCPServers: true}
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
	found, _, err := settingsFindings(pins.Claude{BlockedPlugins: []string{"a@b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 {
		t.Fatal("settings that do not match the pins passed the check")
	}
	// Every finding is claimed by the verb that resolves it, which is what
	// makes a report actionable. It used to be asserted as a sentence — each
	// finding's Fix ending in "fix with: dev session sync" — and a sentence
	// is a fix a reader has to carry out by hand. Now the fixer is declared,
	// so the report can route to it and --fix can run it, and what this holds
	// is that every finding reaches one.
	rep := cli.NewReport("session", "here")
	for _, f := range found {
		rep.Add(f)
	}
	cli.Attribute(rep, fixers())
	for _, f := range rep.Findings {
		if f.FixedBy == "" {
			t.Errorf("%s is claimed by no fixer, so nothing can put it right: %q", f.ID, f.Message)
		}
	}
	// And the fixer that claims it is the one a reader would run.
	if got := cli.Claimed(rep, fixers()); len(got) != 1 || got[0].Name != "sync" {
		t.Errorf("settings drift routes to %v; sync is what writes those keys", got)
	}
}

// A repo that never mentions connectors must not have them turned off behind
// its back, so an absent key writes no setting at all.
func TestConnectorsUnmanagedWhenUnset(t *testing.T) {
	if _, ok := wantSettings(pins.Claude{})["disableClaudeAiConnectors"]; ok {
		t.Error("an unset claude_ai_connectors disabled them anyway")
	}
	off := false
	if want := wantSettings(pins.Claude{ClaudeAIConnectors: &off}); want["disableClaudeAiConnectors"] != true {
		t.Errorf("claude_ai_connectors = false did not disable them: %v", want)
	}
	on := true
	if _, ok := wantSettings(pins.Claude{ClaudeAIConnectors: &on})["disableClaudeAiConnectors"]; ok {
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

// The manual is what a repo reads before it has any of this. A preset dev
// ships that the manual does not name is one nobody can opt into, and the
// manual naming one that does not exist is worse — so the two are held
// together here, where the prose and the catalogue are both in reach.
func TestTheManualNamesEveryPreset(t *testing.T) {
	for name := range pins.Presets {
		if !strings.Contains(Usage, name) {
			t.Errorf("dev ships the %q preset and usage.md never names it", name)
		}
	}
	// Every skill a preset vendors, named where a reader can see what they get.
	for _, preset := range pins.Presets {
		for _, src := range preset.Source {
			if !strings.Contains(Usage, src.Repo) {
				t.Errorf("a preset takes skills from %s and usage.md does not say so", src.Repo)
			}
		}
	}
}
