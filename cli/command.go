package cli

import (
	"cmp"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
	"time"
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

// AgentDirs are the directories an agent reads skills from, in the order a
// message lists them. Every skill a repo has belongs in all of them: an agent
// reads its own and no other, so a skill in one alone is one the rest cannot
// see — which `<cmd> skills` reports and nothing was fixing for skills that
// come from an upstream.
//
// ShippedDir is not one of them. Nothing reads it in place; it is what a
// release carries, and only a command's own manual goes there.
func AgentDirs() []string { return []string{ClaudeDir, AgentsDir} }

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

	// Pin is the packslip path a consumer pins, and PubKey the key its
	// releases are signed with. `<Name> version --pin` prints the line to
	// paste, so the one place that knows how to install this command is the
	// command: a README that writes the key out by hand goes stale the day
	// the key rotates, and nothing tells it.
	Pin    string
	PubKey string
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
	os.Exit(c.Run(os.Args[1:], os.Stdout, os.Stderr))
}

// Run is Main without the exit: it runs the command and returns the code,
// so a test can drive the whole thing — flags parsed as a person's would be,
// arguments held to what each verb declares — and read what came back. Every
// command on the stack gets that from declaring its table.
func (c Command) Run(args []string, stdout, stderr io.Writer) int {
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
		near := ""
		if n := Nearest(verb, SortedKeys(verbs)); n != "" {
			near = fmt.Sprintf("did you mean %q?\n\n", n)
		}
		fmt.Fprintf(stderr, "unknown verb %q\n%s\n%s", verb, near, c.index())
		return 2
	}
	// A subcommand it declares runs itself. Without this a package lists its
	// subcommands in Subs and then names them again in a switch, and the two
	// lists drift — which is a fact stated twice, the thing this stack keeps
	// deleting.
	// spec is what the arguments are parsed against: the verb, or the
	// subcommand once one is matched. A subcommand declares its own Args and
	// Flags, and parsing the parent's instead registered none of them — so
	// `secrets set --env prod` was refused by the binary while the manual,
	// which renders from the same declaration, advertised the flag.
	run, spec, path := v.Run, v, c.Name+" "+verb
	named := ""
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		named = rest[0]
		if sub, ok := v.Subs[named]; ok && sub.Run != nil {
			run, spec, path, rest = sub.Run, sub, path+" "+named, rest[1:]
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
	call, err := spec.parse(path, rest, stdout, stderr)
	if err == nil {
		// Every run is timed. A person wants to know a crawl took nine
		// seconds and a build two, and nobody adds that per verb — so cli
		// does it once, for every command on the stack. It goes to stderr
		// because stdout is the verb's data and may be piped.
		call.Started = time.Now()
		err = run(call)
		if !errors.Is(err, ErrHelp) {
			fmt.Fprintf(stderr, "%s: %s\n", path, Took(call.Elapsed()))
		}
	}
	if errors.Is(err, ErrHelp) {
		// The flag package has printed each flag and what it means; this adds
		// what the verb is for. Together they are the whole of what a person
		// needs, and neither is an error.
		// The group's prose goes with it. Whoever is reading this is here
		// because they are about to run one verb, and an agent does not read
		// the manual first — so what the flags mean is half of it, and why
		// this family of verbs behaves as it does is the other half.
		usage := cmp.Or(one(c.Name, verbs, helpPath(verb, rest)), one(c.Name, verbs, verb))
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
	m["skills"] = Verb{Run: c.skills, Flags: JSONFlags, Desc: "list what every agent in this repo can read, and where each came from", Usage: own}
	m["version"] = Verb{Run: c.version, Flags: pinFlag, Desc: "print the version, or with --pin the line that installs this build", Usage: own}
	m["tools"] = Verb{Run: c.tools, Flags: ToolsFlags, Desc: "every program this command may run, the mise line that installs it, and whether it is here", Usage: own}
	return m
}

//go:embed usage.md
var usageTemplate string

// ownUsage is the usage of skill and version, in the shape the others use.
func (c Command) ownUsage() string { return usageTemplate }

// manualOrder is the verb names in the order the manual reads them: Order
// first, for the groups that earned a place, then the rest by name. A name in
// Order that is not a verb is skipped rather than fatal, so renaming a verb
// degrades to the old ordering instead of breaking the build.
func (c Command) manualOrder(verbs map[string]Verb) []string {
	var names []string
	for _, name := range Unique(c.Order) {
		if _, ok := verbs[name]; ok {
			names = append(names, name)
		}
	}
	listed := ToSet(names)
	for _, name := range SortedKeys(verbs) {
		if !listed[name] {
			names = append(names, name)
		}
	}
	return names
}
