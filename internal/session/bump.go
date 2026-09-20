package session

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
	"github.com/joeblew999/dev/internal/session/pins"
	"github.com/joeblew999/dev/internal/session/vendored"
)

// Bump moves every source this repo declared to upstream HEAD. It shows what
// would change first and waits for confirmation, so moving forward is one
// deliberate command instead of a silent drift.
//
// A source that came from a dev preset is not moved and cannot be: it is not
// in session.toml to rewrite, and it moves when dev is upgraded. Naming one
// says so rather than appearing to work.
func Bump(out io.Writer, sources []string) error {
	p, err := pins.Load()
	if err != nil {
		return err
	}
	names := p.Declared()
	if len(sources) > 0 {
		for _, name := range sources {
			src, known := p.Source[name]
			if !known {
				return fmt.Errorf("%s: %w", pins.File, cli.Unknown("source", name, p.Declared()))
			}
			if preset, fromPreset := src.FromPreset(); fromPreset {
				return fmt.Errorf("%s comes from dev's %q preset, not from %s, so there is no ref here to move; it follows the dev you pin",
					name, preset, pins.File)
			}
		}
		names = sources
	}

	before, err := collect(p)
	if err != nil {
		return err
	}
	moved := false
	for _, name := range names {
		s := p.Source[name]
		head, err := lsRemoteHead(s.Repo)
		if err != nil {
			return err
		}
		if head == s.Ref {
			fmt.Fprintf(out, "%s is already at upstream HEAD (%s)\n", s.Repo, pins.Short(head))
			continue
		}
		fmt.Fprintf(out, "%s: %s -> %s\n", s.Repo, pins.Short(s.Ref), pins.Short(head))
		s.Ref = head
		p.Source[name] = s
		moved = true
	}
	if !moved {
		return nil
	}
	after, err := collect(p)
	if err != nil {
		return err
	}
	for _, line := range cli.DiffMaps(before, after, func(a, b []byte) bool { return string(a) == string(b) }) {
		// The lock changes on every bump by construction — it records the very
		// refs being moved — so saying so tells a reader nothing.
		if strings.HasSuffix(line, ": "+vendored.LockFile) {
			continue
		}
		fmt.Fprintln(out, "  "+line)
	}

	if !confirm(out, "rewrite "+pins.File+"? [y/N] ") {
		return fmt.Errorf("not bumped")
	}
	if err := rewriteRefs(pins.File, p); err != nil {
		return err
	}
	fmt.Fprintf(out, "pinned; run: %s\n", pins.SyncCommand())
	return nil
}

// lsRemoteHead returns upstream HEAD without cloning.
func lsRemoteHead(repo string) (string, error) {
	res, err := tool.Cmd{Bin: GitBin, Args: []string{"ls-remote", "https://github.com/" + repo, "HEAD"}}.Capture()
	out := res.Out
	if err != nil {
		return "", fmt.Errorf("git ls-remote https://github.com/%s HEAD: %w", repo, err)
	}
	head, _, ok := strings.Cut(out, "\t")
	if !ok || head == "" {
		return "", fmt.Errorf("git ls-remote https://github.com/%s HEAD answered %q", repo, strings.TrimSpace(out))
	}
	return head, nil
}

func confirm(out io.Writer, prompt string) bool {
	fmt.Fprint(out, prompt)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

// rewriteRefs replaces the ref line of every declared source in session.toml,
// keeping comments and order. A preset source has no line here to find, which
// is why it cannot be bumped.
func rewriteRefs(path string, p pins.Pins) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var out []string
	var section string
	for _, line := range cli.Lines(string(data)) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.Trim(trimmed, "[]")
		}
		if name, ok := strings.CutPrefix(section, "source."); ok {
			if s, known := p.Source[name]; known && strings.HasPrefix(trimmed, "ref") {
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				line = indent + `ref = "` + s.Ref + `"`
			}
		}
		out = append(out, line)
	}
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}
