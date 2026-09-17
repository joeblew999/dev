package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
)

// VerbMarker is the line in a command's Skill that the rendered verbs
// replace. An HTML comment, so a manual with no verbs yet still reads as
// markdown and the marker does not show.
const VerbMarker = "<!-- verbs -->"

// Where a command's manual lives: the directory the release ships, and the
// directories each agent reads in its own repo. One render, one file under
// each; paths() joins them with the command's name.
const (
	ShippedDir = "skills"         // what `dev release` ships
	ClaudeDir  = ".claude/skills" // what Claude Code reads
	AgentsDir  = ".agents/skills" // what Copilot reads
	SkillFile  = "SKILL.md"
)

// Command is a whole binary: its verbs, and the manual rendered from them.
// Main runs it; CheckSkill holds its manual to its verbs from a test. Every
// command gets skill and version without writing them: `<name> skill` writes
// one file under each of ShippedDir, ClaudeDir and AgentsDir, and `--check`
// fails naming whichever copy is stale.
type Command struct {
	Name    string          // the binary's name: skills/<Name>/SKILL.md is its manual
	Verbs   map[string]Verb // the table; skill and version are added to it
	Default string          // the verb run with no verb given; "" prints the index
	Version string          // what `<Name> version` prints
	// Skill is the manual a person writes: frontmatter, prose, and a
	// VerbMarker line where the rendered verbs belong. One document, because
	// it is one document — a Head and a Tail made every sentence a question
	// about which half it went in, and the answer was only ever "wherever the
	// generated part is not".
	Skill string

	// Skills are manuals this command ships that are not its own verbs: a
	// library it exposes, named by what a reader would look for. `dev` ships
	// one for this package, because a repo writing a command against it needs
	// the library's rules, not dev's verbs, and had nowhere to read them.
	//
	// Each is written to skills/<name>/ beside the command's, so `dev release`
	// ships it with no new plumbing, and CheckSkill holds it like any other.
	Skills map[string]string

	// Order is the manual's reading order: verb names, each standing for the
	// group that shares its usage. Verbs is a map and has no order of its
	// own, so without this the manual is whatever alphabetical accident the
	// verb names make — `init` fifth, when it is the first thing anyone does.
	// Unlisted verbs follow in name order, so leaving it empty renders
	// exactly as before it existed.
	Order []string
}

// Main runs c as a program and exits: 0 on success, 1 on an error, 2 on a
// usage error, an unknown verb, or no verb when c names no Default.
func Main(c Command) {
	os.Exit(c.run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is Main without the exit, so a test can see the code.
func (c Command) run(args []string, stdout, stderr io.Writer) int {
	verbs := c.all()
	if c.Default != "" && (len(args) == 0 || strings.HasPrefix(args[0], "-")) {
		args = append([]string{c.Default}, args...)
	}
	// Asking what the command does is not an error, so it goes to stdout and
	// exits 0 — the same rule a verb's --help follows. With no verb at all the
	// index is a correction rather than an answer: it goes to stderr and
	// exits 2, because something was meant to run and did not.
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(stdout, c.index())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprint(stderr, c.index())
		return 2
	}
	verb, rest := args[0], args[1:]
	v, ok := verbs[verb]
	if !ok {
		fmt.Fprintf(stderr, "unknown verb %q\n\n%s", verb, c.index())
		return 2
	}
	// A subcommand it declares runs itself. Without this a package lists its
	// subcommands in Subs and then names them again in a switch, and the two
	// lists drift — which is a fact stated twice, the thing this stack keeps
	// deleting.
	run, path := v.Run, c.Name+" "+verb
	named := ""
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		named = rest[0]
		if sub, ok := v.Subs[named]; ok && sub.Run != nil {
			run, path, rest = sub.Run, path+" "+named, rest[1:]
		}
	}
	if run == nil {
		// Say which of the two it was. "takes a subcommand" is true when none
		// was given and a lie when one was, and the second is the case where
		// someone is already looking at the wrong word.
		what := fmt.Sprintf("%s takes a subcommand", path)
		if named != "" {
			what = fmt.Sprintf("%s has no subcommand %q", path, named)
		}
		fmt.Fprintf(stderr, "error: %s\n\n%s", what, Flatten(c.sectionFor(verb)))
		return 2
	}
	call, err := v.parse(path, rest, stdout, stderr)
	if err == nil {
		err = run(call)
	}
	if errors.Is(err, ErrHelp) {
		// The flag package has printed each flag and what it means; this adds
		// what the verb is for. Together they are the whole of what a person
		// needs, and neither is an error.
		// The group's prose goes with it. Whoever is reading this is here
		// because they are about to run one verb, and an agent does not read
		// the manual first — so what the flags mean is half of it, and why
		// this family of verbs behaves as it does is the other half.
		usage := one(c.Name, verbs, helpPath(verb, rest))
		if usage == "" {
			usage = one(c.Name, verbs, verb)
		}
		if prose := strings.TrimRight(v.Usage, "\n"); prose != "" {
			usage = prose + "\n\n" + usage
		}
		fmt.Fprintf(stdout, "\n%s", Flatten(usage))
		return 0
	}
	if _, ok := errors.AsType[*UsageError](err); ok {
		// The group, not just its prose. The arguments were wrong, so what is
		// wanted is the verbs — `dev deps` used to answer "list or upgrade"
		// and then print prose naming neither, because a usage.md stopped
		// carrying the verbs when they started being rendered.
		fmt.Fprintf(stderr, "error: %v\n\n%s", err, Flatten(c.sectionFor(verb)))
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

// helpPath is what a help request is about: the verb, and any subcommand
// after it. A verb's arguments and its subcommands look alike from here —
// "secrets push" is a subcommand, "check ." is a verb and a directory — so
// the caller tries this and falls back to the verb alone.
func helpPath(verb string, rest []string) string {
	var path strings.Builder
	path.WriteString(verb)
	for _, arg := range rest {
		if arg == "--" || strings.HasPrefix(arg, "-") {
			break
		}
		path.WriteString(" " + arg)
	}
	return path.String()
}

// all is the table plus the two verbs every command has.
func (c Command) all() map[string]Verb {
	m := make(map[string]Verb, len(c.Verbs)+2)
	maps.Copy(m, c.Verbs)
	own := c.ownUsage()
	m["skill"] = Verb{Run: c.skill, Flags: checkFlag, Desc: "rewrite the manual from the verbs, in all three places it is read", Usage: own}
	m["skills"] = Verb{Run: c.skills, Desc: "list what every agent in this repo can read, and where each came from", Usage: own}
	m["version"] = Verb{Run: c.version, Desc: "print the version, to tell a release from a local build", Usage: own}
	return m
}

//go:embed usage.md
var usageTemplate string

// ownUsage is the usage of skill and version, in the shape the others use.
func (c Command) ownUsage() string { return usageTemplate }

// sortedVerbs is the table's names in order. Verbs is a map, so it has none
// of its own; everything that walks the table walks it through here, so the
// manual, the index and CheckUsage all agree.
func sortedVerbs(verbs map[string]Verb) []string {
	names := make([]string, 0, len(verbs))
	for name := range verbs {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// manualOrder is the verb names in the order the manual reads them: Order
// first, for the groups that earned a place, then the rest by name. A name in
// Order that is not a verb is skipped rather than fatal, so renaming a verb
// degrades to the old ordering instead of breaking the build.
func (c Command) manualOrder(verbs map[string]Verb) []string {
	var names []string
	listed := map[string]bool{}
	for _, name := range c.Order {
		if _, ok := verbs[name]; ok && !listed[name] {
			listed[name] = true
			names = append(names, name)
		}
	}
	for _, name := range sortedVerbs(verbs) {
		if !listed[name] {
			names = append(names, name)
		}
	}
	return names
}
