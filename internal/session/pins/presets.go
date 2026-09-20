package pins

import (
	"fmt"
	"strings"

	"github.com/joeblew999/dev/cli"
)

// The skill sets dev keeps for the stack.
//
// A repo that wants them writes one line — `presets = ["recommended"]` — and
// gets what dev currently says is good, at the commits dev pins. It decides to
// take them; dev decides what they are. Both halves matter: a repo that never
// asks gets nothing, and a repo that asks does not also have to track four
// upstream commits by hand.
//
// A preset moves when dev is released, so a repo's skills move when it upgrades
// dev and at no other time. That is the same shape as every other pin on the
// stack, which is why the refs here are full commits like any other source and
// a test holds them to the same rule.

// Preset is one named set: why it exists, and the upstreams it draws from.
type Preset struct {
	Why    string
	Source map[string]Source
}

// Presets is the whole catalogue. It is small on purpose — each of these has
// been synced and checked end to end, and a skill earns its place by being one
// this stack actually works the way of, not by existing.
var Presets = map[string]Preset{
	"recommended": {
		Why: "the ones this stack works the way of: test-first, debug by evidence, and write for the agent that reads it",
		Source: map[string]Source{
			"mattpocock": {
				Repo: "mattpocock/skills",
				Ref:  "c55ee46073ed923f86ce59a5eb3b6d895095d1b7",
				Skills: []string{
					"engineering/tdd",
					"productivity/writing-for-agents",
				},
			},
			"superpowers": {
				Repo: "obra/superpowers",
				Ref:  "5bf4e78011075bcfc0dc295f0724994cd123ee71",
				Skills: []string{
					"systematic-debugging",
					"brainstorming",
				},
			},
		},
	},
}

// expand folds every preset the repo asked for into Source, so that everything
// downstream sees one flat set and never has to know which half dev supplied.
// A name a preset would take that the repo already uses is refused rather than
// resolved: silently preferring either one gives a repo skills it did not ask
// for under a name it did.
func (p *Pins) expand() error {
	for _, want := range p.Presets {
		preset, ok := Presets[want]
		if !ok {
			return fmt.Errorf("%s: no preset named %q; dev ships %s",
				File, want, cli.Or(english(cli.SortedKeys(Presets)), "none"))
		}
		for _, name := range cli.SortedKeys(preset.Source) {
			if _, taken := p.Source[name]; taken {
				return fmt.Errorf("%s: [source.%s] has the name preset %q uses; rename yours",
					File, name, want)
			}
			src := preset.Source[name]
			src.preset = want
			if p.Source == nil {
				p.Source = map[string]Source{}
			}
			p.Source[name] = src
		}
	}
	return nil
}

// english joins names the way a sentence does.
func english(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return fmt.Sprintf("%s and %s", joinAll(names[:len(names)-1]), names[len(names)-1])
}

func joinAll(names []string) string {
	var out strings.Builder
	for i, n := range names {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(n)
	}
	return out.String()
}
