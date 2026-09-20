// Package pins is what a repo declares about its Claude Code session.
//
// Its one source of truth is session.toml, a committed file. Nothing here
// touches the network or the filesystem beyond reading that file: a pin is a
// statement of intent, and whether the world matches it is somebody else's
// question. That boundary is the point — this package can be wrong only about
// what the repo said, never about what happened.
package pins

import (
	"fmt"
	"path"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/conf"
)

// File is where a repo writes all of this.
const File = "session.toml"

// Pins is the whole declaration: the presets it takes from dev, the sources it
// names itself, and the part of the Claude Code session it owns.
type Pins struct {
	// Presets are named sets dev keeps, taken at the commits this dev pins.
	Presets []string          `toml:"presets"`
	Source  map[string]Source `toml:"source"`
	Claude  Claude            `toml:"claude"`
	// SyncCommand is how this repo prefers sync to be run, quoted back in
	// every error a sync would fix. A repo whose front door is a task runner
	// sets it to that task; left empty, errors name the binary.
	SyncCommand string `toml:"sync_command"`
}

// Claude is the part of the Claude Code session this repo owns: which
// marketplace plugins must not load, and where MCP servers come from. Sync
// writes it into .claude/settings.json; check fails when the two disagree.
type Claude struct {
	BlockedPlugins []string `toml:"blocked_plugins"`
	// ClaudeAIConnectors is a pointer so that leaving it out means "not this
	// repo's business" rather than "off": a repo adopting session must not
	// silently lose its connectors by not mentioning them.
	ClaudeAIConnectors *bool `toml:"claude_ai_connectors"`
	ApproveMCPServers  bool  `toml:"approve_mcp_servers"`
}

// Source is one GitHub repo at one commit, and the skills taken from it. A
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
type Source struct {
	Repo   string   `toml:"repo"`
	Ref    string   `toml:"ref"`
	Dir    string   `toml:"dir"`
	Skills []string `toml:"skills"`

	// preset names the dev preset this source came from, and is empty for one
	// the repo wrote. Bump moves pins in the file and must not pretend to move
	// a pin that is compiled into dev.
	preset string
}

// FromPreset reports whether dev supplied this source rather than the repo.
func (s Source) FromPreset() (string, bool) { return s.preset, s.preset != "" }

// SkillsDir is where this source keeps its skills, defaulting to the
// convention. A leading or trailing slash is the same directory, not an error.
func (s Source) SkillsDir() string { return strings.Trim(cli.Or(s.Dir, "skills"), "/") }

// Prefix is the path every one of this source's files starts with inside the
// tarball GitHub serves: codeload names the top directory after the repo, not
// after `skills` — which is what an upstream not called `skills` used to trip
// over, silently, as "skill not found".
func (s Source) Prefix() string {
	top := s.Repo[strings.LastIndex(s.Repo, "/")+1:]
	if d := s.SkillsDir(); d != "" {
		return fmt.Sprintf("%s-%s/%s/", top, s.Ref, d)
	}
	return fmt.Sprintf("%s-%s/", top, s.Ref)
}

// Within is where a named skill sits inside the source repo, for a message
// that has to say what was looked for.
func (s Source) Within(name string) string {
	return path.Join(s.SkillsDir(), strings.Trim(name, "/"))
}

// At names this source the way a lock records it.
func (s Source) At() string { return fmt.Sprintf("github.com/%s@%s", s.Repo, Short(s.Ref)) }

// Short abbreviates a commit for a message, and never panics on one that is
// not a commit — which validation refuses, but messages are written before
// validation gets a chance.
func Short(ref string) string {
	if len(ref) > 12 {
		return ref[:12]
	}
	return ref
}

// Vendored is the name a skill is written under: the last element of the name
// it has upstream. `engineering/tdd` is `tdd` here, because an agent finds a
// skill one level below .claude/skills and nowhere else.
func Vendored(name string) string { return path.Base(strings.Trim(name, "/")) }

// Names returns source names in sorted order, so sync and check are stable.
func (p Pins) Names() []string { return cli.SortedKeys(p.Source) }

// Declared is the sources the repo wrote itself, which are the only ones bump
// may move: the rest are dev's, and they move when dev does.
func (p Pins) Declared() []string {
	return cli.Filter(p.Names(), func(name string) bool {
		_, fromPreset := p.Source[name].FromPreset()
		return !fromPreset
	})
}

// Load reads session.toml and expands the presets it asks for. Every error
// names the fix: the file is the only place a skill or a pin is named. Unknown
// keys fail — a typo must not silently drop a source.
func Load() (Pins, error) {
	p, unknown, err := conf.LoadStrict[Pins](File)
	if err != nil {
		return Pins{}, fmt.Errorf("%w; fix the file, then: %s", err, SyncCommand())
	}
	if len(unknown) > 0 {
		return Pins{}, fmt.Errorf("%s: unknown key %q; fix the file, then: %s", File, unknown[0], SyncCommand())
	}
	if err := p.expand(); err != nil {
		return Pins{}, err
	}
	for _, name := range p.Names() {
		if err := p.Source[name].check(name); err != nil {
			return Pins{}, err
		}
	}
	return p, nil
}

// check holds one source to what a pin has to be. It is a method so that a
// preset compiled into dev is held to exactly what a repo's own source is —
// one rule, and a test runs it over every preset dev ships.
func (s Source) check(name string) error {
	switch {
	case s.Repo == "" || s.Ref == "":
		return fmt.Errorf("%s: [source.%s] needs repo and ref; fix the file, then: %s", File, name, SyncCommand())
	case strings.Count(s.Repo, "/") != 1:
		return fmt.Errorf("%s: [source.%s] repo is %q; it wants owner/name, which is what GitHub is asked for", File, name, s.Repo)
	case !isCommit(s.Ref):
		// A branch or a tag is not a pin: it is a name upstream may point
		// anywhere tomorrow, and a repo that syncs from one gets a different
		// session on a different day with the file unchanged. `bump` is how a
		// pin moves, and it writes the commit it moved to.
		return fmt.Errorf("%s: [source.%s] ref is %q, which is a name and not a commit, so it pins nothing; use the full commit sha — `dev session bump %s` writes upstream's", File, name, s.Ref, name)
	case len(s.Skills) == 0:
		return fmt.Errorf("%s: [source.%s] lists no skills", File, name)
	}
	return nil
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
