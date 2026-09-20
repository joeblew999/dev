package session

import (
	"fmt"
	"path"
	"strings"

	"github.com/joeblew999/dev/internal/conf"

	"github.com/joeblew999/dev/cli"
)

// pins is everything session.toml says: named sources, each a GitHub repo at
// a pinned commit, and the part of the Claude Code session this repo owns.
type pins struct {
	Source map[string]sourcePins `toml:"source"`
	Claude claudePins            `toml:"claude"`
	// SyncCommand is how this repo prefers sync to be run, quoted back in
	// every error a sync would fix. A repo whose front door is a task runner
	// sets it to that task; left empty, errors name this binary.
	SyncCommand string `toml:"sync_command"`
}

// claudePins is the part of the Claude Code session this repo owns: which
// marketplace plugins must not load, and where MCP servers come from. Sync
// writes it into .claude/settings.json; check fails when the two disagree.
type claudePins struct {
	BlockedPlugins []string `toml:"blocked_plugins"`
	// ClaudeAIConnectors is a pointer so that leaving it out means "not this
	// repo's business" rather than "off": a repo adopting session must not
	// silently lose its connectors by not mentioning them.
	ClaudeAIConnectors *bool `toml:"claude_ai_connectors"`
	ApproveMCPServers  bool  `toml:"approve_mcp_servers"`
}

// sourcePins is one GitHub repo at one commit, and the skills taken from it. A
// commit, not a tag: an upstream that ships skills usually ships no releases,
// and a tool that does ships a packslip release, which mise pins and links
// without any help from here.
//
// Dir is where skills live in that repo, `skills` unless it says otherwise —
// the convention, not a rule, and a repo that keeps them at its root or under
// a category says so here rather than being unusable.
//
// A name in Skills may be a path within Dir, because upstreams file them by
// category: `engineering/tdd` is taken from there and vendored as `tdd`, since
// Claude Code reads `.claude/skills/<name>/SKILL.md` and looks no deeper.
type sourcePins struct {
	Repo   string   `toml:"repo"`
	Ref    string   `toml:"ref"`
	Dir    string   `toml:"dir"`
	Skills []string `toml:"skills"`
}

// isCommit reports whether ref is a full git object name: forty hex digits.
// Anything shorter is either a name that moves or an abbreviation that stops
// being unique as upstream grows.
func isCommit(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	return strings.IndexFunc(ref, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f')
	}) < 0
}

// dir is where this source keeps its skills, defaulting to the convention.
// A leading or trailing slash is the same directory, so it is not an error.
func (s sourcePins) dir() string { return strings.Trim(cli.Or(s.Dir, "skills"), "/") }

// prefix is the path every one of this source's files starts with inside the
// tarball GitHub serves: codeload names the top directory after the repo, not
// after `skills` — which is what an upstream not called `skills` used to trip
// over, silently, as "skill not found".
func (s sourcePins) prefix() string {
	top := s.Repo[strings.LastIndex(s.Repo, "/")+1:]
	if d := s.dir(); d != "" {
		return fmt.Sprintf("%s-%s/%s/", top, s.Ref, d)
	}
	return fmt.Sprintf("%s-%s/", top, s.Ref)
}

// vendored is the name a skill is written under: the last element of the name
// it has upstream. `engineering/tdd` is `tdd` here, because an agent finds a
// skill one level below .claude/skills and nowhere else.
func vendored(name string) string {
	return path.Base(strings.Trim(name, "/"))
}

// loadPins reads session.toml. Every error names the fix: the file is the only
// place a skill or a pin is named. Unknown keys fail: a typo must not silently
// drop a source.
func loadPins() (pins, error) {
	p, unknown, err := conf.LoadStrict[pins](pinsFile)
	if err != nil {
		return pins{}, fmt.Errorf("%w; fix the file, then: "+syncCmd, err)
	}
	if len(unknown) > 0 {
		return pins{}, fmt.Errorf("%s: unknown key %q; fix the file, then: "+syncCmd, pinsFile, unknown[0])
	}
	names := p.names()
	for _, name := range names {
		s := p.Source[name]
		if s.Repo == "" || s.Ref == "" {
			return pins{}, fmt.Errorf("%s: [source.%s] needs repo and ref; fix the file, then: "+syncCmd, pinsFile, name)
		}
		if strings.Count(s.Repo, "/") != 1 {
			return pins{}, fmt.Errorf("%s: [source.%s] repo is %q; it wants owner/name, which is what GitHub is asked for", pinsFile, name, s.Repo)
		}
		// A branch or a tag is not a pin: it is a name upstream may point
		// anywhere tomorrow, and a repo that syncs from one gets a different
		// session on a different day with the file unchanged. `bump` is how a
		// pin moves, and it writes the commit it moved to.
		if !isCommit(s.Ref) {
			return pins{}, fmt.Errorf("%s: [source.%s] ref is %q, which is a name and not a commit, so it pins nothing; use the full commit sha — `dev session bump %s` writes upstream's", pinsFile, name, s.Ref, name)
		}
		if len(s.Skills) == 0 {
			return pins{}, fmt.Errorf("%s: [source.%s] lists no skills", pinsFile, name)
		}
	}
	if p.SyncCommand != "" {
		syncCmd = p.SyncCommand
	}
	return p, nil
}

// names returns source names in sorted order, so sync and check are stable.
func (p pins) names() []string {
	return cli.SortedKeys(p.Source)
}
