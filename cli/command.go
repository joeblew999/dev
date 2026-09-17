package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Where a command's manual lives: the directory the release ships, and the
// directories each agent reads in its own repo. One render, one file under
// each; paths() joins them with the command's name.
const (
	ShippedDir = "skills"         // what `dev release` ships
	ClaudeDir  = ".claude/skills" // what Claude Code reads
	AgentsDir  = ".agents/skills" // what Copilot reads
	SkillFile  = "SKILL.md"
)

// Verb is one verb of a command: what runs, and the usage the manual shows.
// Verbs that share one Usage (build, wasm and check are all stage's) share it
// verbatim, and the index and the manual print it once.
type Verb struct {
	Run   Runner
	Usage string
}

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
	Head    string          // the manual's frontmatter and prose before the verbs
	Tail    string          // the prose after

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
	err := v.Run(verb, rest, stdout, stderr)
	if errors.Is(err, ErrHelp) {
		// The flag package has printed each flag and what it means; this adds
		// what the verb is for. Together they are the whole of what a person
		// needs, and neither is an error.
		usage := v.Usage
		if entry := Entry(usage, c.Name, verb); entry != "" {
			usage = entry
		}
		fmt.Fprintf(stdout, "\n%s", Flatten(usage))
		return 0
	}
	var uerr *UsageError
	if errors.As(err, &uerr) {
		fmt.Fprintf(stderr, "error: %v\n\n%s", err, Flatten(v.Usage))
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

// all is the table plus the two verbs every command has.
func (c Command) all() map[string]Verb {
	m := make(map[string]Verb, len(c.Verbs)+2)
	for name, v := range c.Verbs {
		m[name] = v
	}
	own := c.ownUsage()
	m["skill"] = Verb{c.skill, own}
	m["version"] = Verb{c.version, own}
	return m
}

// usageTemplate is the usage of skill and version: this package's own
// usage.md, named as every package on the stack names it. It is a template
// rather than a plain file because it has to say the binary's name and the
// three paths, which only the command knows.

//go:embed usage.md
var usageTemplate string

// ownUsage is the usage of skill and version, in the shape the others use.
func (c Command) ownUsage() string {
	return fmt.Sprintf(usageTemplate, c.Name,
		filepath.Join(ShippedDir, c.Name, SkillFile),
		filepath.Join(ClaudeDir, c.Name, SkillFile),
		filepath.Join(AgentsDir, c.Name, SkillFile))
}

// sortedVerbs is the table's names in order. Verbs is a map, so it has none
// of its own; everything that walks the table walks it through here, so the
// manual, the index and CheckUsage all agree.
func sortedVerbs(verbs map[string]Verb) []string {
	names := make([]string, 0, len(verbs))
	for name := range verbs {
		names = append(names, name)
	}
	sort.Strings(names)
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

// usages is every distinct usage, in the manual's order, each once.
func (c Command) usages() []string {
	verbs := c.all()
	names := c.manualOrder(verbs)
	seen := map[string]bool{}
	var out []string
	for _, name := range names {
		u := verbs[name].Usage
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// index is what the binary prints with no verb: every usage, flattened,
// because a terminal has no markdown renderer.
func (c Command) index() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: verbs, by what does them:\n\n", c.Name)
	for _, u := range c.usages() {
		b.WriteString(Flatten(u))
		b.WriteString("\n")
	}
	return b.String()
}

// render is the manual: Head, every usage, Tail. Markdown usage goes in as
// the markdown it is, so its headings, inline code and lists are the manual's
// own. Plain text is fenced, as every usage was before this package read
// markdown.
//
// That is what keeps a repo that has not ported yet correct. Its usage is
// hand-aligned columns with no markdown in it, and unfenced a renderer
// collapses the alignment, runs every verb into one paragraph and eats any
// `<placeholder>` as an HTML tag — a manual quietly made worse by upgrading.
// Fencing legacy text means a repo ports when it chooses rather than when it
// bumps a pin.
func (c Command) render() string {
	var b strings.Builder
	b.WriteString(c.Head)
	for _, u := range c.usages() {
		if isMarkdown(u) {
			b.WriteString(strings.TrimRight(u, "\n") + "\n\n")
		} else {
			b.WriteString("```\n" + u + "```\n\n")
		}
	}
	b.WriteString(c.Tail)
	return b.String()
}

// isMarkdown reports whether a usage string uses the markdown shape — a
// heading or a list item. Anything else is the plain text this package took
// before, and is rendered the way that text has always been rendered.
func isMarkdown(usage string) bool {
	for _, line := range strings.Split(usage, "\n") {
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "#") {
			return true
		}
	}
	return false
}

// paths are the manual's copies: the one the release ships, and the ones
// agents read in this repo — Claude Code's .claude/skills and Copilot's
// .agents/skills. All sit under the repo root, found by walking up to
// mise.toml — every repo on the stack has one at its root — and never
// through git: a Worker's main links this package into its wasm.
func (c Command) paths() (shipped, claude, agents string, err error) {
	root, err := root(".")
	if err != nil {
		return "", "", "", err
	}
	return filepath.Join(root, ShippedDir, c.Name, SkillFile),
		filepath.Join(root, ClaudeDir, c.Name, SkillFile),
		filepath.Join(root, AgentsDir, c.Name, SkillFile), nil
}

// root is the nearest directory at or above dir holding a mise.toml.
func root(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, "mise.toml")); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("no mise.toml at or above %s; run this inside a repo on the stack", dir)
		}
		abs = parent
	}
}

// rel is p as a message shows it: relative to where the command runs.
func rel(p string) string {
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, p); err == nil {
			return r
		}
	}
	return p
}

// skill is `<Name> skill [--check]`: write every copy of the manual, or with
// --check say which is stale and how to fix it.
func (c Command) skill(verb string, args []string, stdout, stderr io.Writer) error {
	fs := Flags(verb, stderr)
	var check Bool
	fs.Var(&check, "check", "fail when any copy is stale; write nothing")
	rest, err := ParseInterleaved(fs, args)
	if errors.Is(err, ErrHelp) {
		return err
	}
	if err != nil {
		return Usagef("%s: %v", verb, err)
	}
	if len(rest) > 0 {
		return Usagef("%s skill takes only --check", c.Name)
	}
	// Both branches below answer from prose compiled into this binary, so
	// neither means anything if the binary is behind its sources: writing
	// would rewrite every copy from old bytes and report success, and
	// checking would compare that old render against equally old files and
	// report "up to date". Refuse instead — the cost is one rebuild, and the
	// alternative is a wrong answer nobody can see.
	dir, err := root(".")
	if err != nil {
		return err
	}
	if changed := staleBuild(dir); changed != "" {
		what := "write a manual"
		if check {
			what = "check a manual"
		}
		return fmt.Errorf("%s has changed since this %s was built, so it would %s from stale embedded prose; rebuild first: mise run build", rel(changed), c.Name, what)
	}
	want := c.render()
	shipped, claude, agents, err := c.paths()
	if err != nil {
		return err
	}
	for _, p := range []string{shipped, claude, agents} {
		if check {
			have, err := os.ReadFile(p)
			if err != nil || string(have) != want {
				return fmt.Errorf("%s is stale; regenerate it with: %s skill", rel(p), c.Name)
			}
			fmt.Fprintf(stdout, "%s is up to date\n", rel(p))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(want), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "wrote %s from the verbs' own usage\n", rel(p))
	}
	return nil
}

// version is `<Name> version`.
func (c Command) version(verb string, args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		return Usagef("%s version takes no arguments", c.Name)
	}
	fmt.Fprintln(stdout, c.Version)
	return nil
}

// TB is the part of testing.TB that CheckSkill needs, so that importing this
// package never links the testing package into a binary.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
}

// CheckSkill fails the test when any copy of c's manual differs from what
// its verbs render. A command's main_test.go calls it, so `go test` — and so
// `dev check` — holds every manual to its verbs.
func CheckSkill(t TB, c Command) {
	t.Helper()
	want := c.render()
	shipped, claude, agents, err := c.paths()
	if err != nil {
		t.Errorf("%v", err)
		return
	}
	for _, p := range []string{shipped, claude, agents} {
		have, err := os.ReadFile(p)
		if err != nil || string(have) != want {
			t.Errorf("%s is stale; regenerate it with: %s skill", rel(p), c.Name)
		}
	}
}
