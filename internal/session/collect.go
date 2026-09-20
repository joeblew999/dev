package session

import (
	"fmt"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/session/pins"
	"github.com/joeblew999/dev/internal/session/upstream"
	"github.com/joeblew999/dev/internal/session/vendored"
)

// collect turns what the repo declared into the files that go on disk. It is
// the one place the three halves meet — pins says what, upstream holds it,
// vendored takes it — and it is deliberately the only place, so that each of
// them can be read without the other two in mind.
func collect(p pins.Pins) (vendored.Set, error) {
	files := vendored.Set{}
	var lock []string
	// Which pin each vendored name came from, so two sources claiming one
	// name is refused by naming both rather than by the second quietly
	// overwriting the first.
	from := map[string]string{}

	for _, name := range p.Names() {
		src := p.Source[name]
		archive, err := upstream.Get(src.Repo, src.Ref)
		if err != nil {
			return nil, err
		}
		for _, skill := range src.Skills {
			as := pins.Vendored(skill)
			if was, taken := from[as]; taken {
				return nil, fmt.Errorf("%s: two skills would both be vendored as %q — %s and %s; one of them has to go",
					pins.File, as, was, name+"/"+skill)
			}
			within, err := locate(archive, src, name, skill)
			if err != nil {
				return nil, err
			}
			if !archive.Dir(files, within, as) {
				return nil, fmt.Errorf("%s: [source.%s] pins skill %q, and %s holds no %s/; check the name, then: %s",
					pins.File, name, skill, src.At(), within, pins.SyncCommand())
			}
			from[as] = name + "/" + skill
			lock = append(lock, vendored.LockSkill(as, src.At(), files))
		}
	}
	files[vendored.LockFile] = []byte(strings.Join(cli.Sorted(lock), "\n") + "\n")
	return files, nil
}

// locate is where a pinned skill actually sits in the upstream repo, and the
// message when it is nowhere.
//
// Three things are tried, in the order of how much they are worth trusting: a
// name that is already a path within the source's dir, then the repository's
// own plugin manifest, then every directory in it that holds a SKILL.md. The
// last one is what makes this work for any upstream at all — a skill is a
// directory with a SKILL.md in it, whatever the repository calls its folders
// and whether or not it ships as a plugin.
//
// So a repo can pin `tdd` without knowing that mattpocock files it under
// engineering, or that superpowers does not file it anywhere.
func locate(a *upstream.Archive, src pins.Source, source, skill string) (string, error) {
	if within := src.Within(skill); a.Has(within) {
		return within, nil
	}
	if at, ok := a.Find(skill); ok {
		return at, nil
	}
	// Nowhere. The message says what this upstream does have, because "not
	// found" on its own sends a reader to check a name that is usually right.
	return "", fmt.Errorf("%s: [source.%s] pins skill %q, and %s has none of that name; %s; then: %s",
		pins.File, source, skill, src.At(), offer(a, skill), pins.SyncCommand())
}

// offer is the nearest name upstream really has, or a count when nothing is
// close — either is more use than silence.
func offer(a *upstream.Archive, skill string) string {
	have := cli.Map(a.Skills(), func(at string) string { return pins.Vendored(at) })
	if len(have) == 0 {
		return "it holds no SKILL.md anywhere, so it ships no skills in a form an agent reads"
	}
	if near := cli.Nearest(pins.Vendored(skill), have); near != "" {
		return "the nearest it has is " + near
	}
	return fmt.Sprintf("it has %s, none like it — %s lists them",
		cli.Plural(len(have), "skill"), "https://github.com/"+a.Repo)
}
