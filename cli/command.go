package cli

import (
	_ "embed"
	"errors"
	"flag"
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

// Verb is one verb of a command: what runs, what it takes, and what it is
// for.
//
// A verb's signature is rendered from Args and Flags, never written: the
// flags are registered in Go already, so typing them into a manual too is a
// second copy of one fact, and the copies drift. `dev release --rotate` was
// registered and named in no manual at all until someone read both by hand.
// Args and Subs are written because nothing else knows them — a positional
// and a subcommand are declared nowhere in a FlagSet.
//
// Usage is therefore the description and nothing else: what the verb does,
// which no code can tell you.
type Verb struct {
	Run   Runner
	Args  string              // the positionals: "DIR", "URL", "DIR [VERSION]"
	Flags func(*flag.FlagSet) // registers them; Run calls it too, so there is one registration
	Desc  string              // one line: what this verb is for, next to the flags it takes
	Usage string              // the group's prose: why these verbs exist, what they share
	Subs  map[string]Verb     // secrets set, deps list: each with its own Args and Flags
}

// Signature is how a verb is written in a manual and in help: the command, the
// verb, its positionals, then every flag it registers, in name order.
//
// This is check I7 of the i18n plan made real — "the rendered skill carries
// every flag in the verb table" — by rendering from the registration rather
// than from prose beside it.
func (v Verb) Signature(name, verb string) string {
	var b strings.Builder
	b.WriteString(name + " " + verb)
	if v.Args != "" {
		b.WriteString(" " + v.Args)
	}
	for _, f := range v.flagSpecs() {
		b.WriteString(" " + f)
	}
	return b.String()
}

// flagSpecs is each registered flag as a manual writes it — [--name VALUE] —
// in name order, which is the order a FlagSet visits them.
func (v Verb) flagSpecs() []string {
	if v.Flags == nil {
		return nil
	}
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	v.Flags(fs)
	var out []string
	fs.VisitAll(func(f *flag.Flag) {
		out = append(out, "[--"+f.Name+placeholder(f)+"]")
	})
	return out
}

// placeholder is what a flag takes, or "" when it takes nothing. A bool is
// written bare, because writing [--yes BOOL] would invite someone to type it.
//
// The name comes from the flag's own help text, through the standard
// library's convention: a backquoted word there names the value, and
// PrintDefaults strips the quotes. So `--env` reading "wrangler `NAME`"
// prints as [--env NAME] here and as "-env NAME" under --help, from one
// string. Without backquotes the type is used, which is what the standard
// library does too.
func placeholder(f *flag.Flag) string {
	if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
		return ""
	}
	name, _ := flag.UnquoteUsage(f)
	if name == "" {
		return ""
	}
	return " " + name
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
	err := v.Run(verb, rest, stdout, stderr)
	if errors.Is(err, ErrHelp) {
		// The flag package has printed each flag and what it means; this adds
		// what the verb is for. Together they are the whole of what a person
		// needs, and neither is an error.
		usage := one(c.Name, verbs, helpPath(verb, rest))
		if usage == "" {
			usage = one(c.Name, verbs, verb)
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

// helpPath is what a help request is about: the verb, and any subcommand
// after it. A verb's arguments and its subcommands look alike from here —
// "secrets push" is a subcommand, "check ." is a verb and a directory — so
// the caller tries this and falls back to the verb alone.
func helpPath(verb string, rest []string) string {
	path := verb
	for _, arg := range rest {
		if arg == "--" || strings.HasPrefix(arg, "-") {
			break
		}
		path += " " + arg
	}
	return path
}

// all is the table plus the two verbs every command has.
func (c Command) all() map[string]Verb {
	m := make(map[string]Verb, len(c.Verbs)+2)
	for name, v := range c.Verbs {
		m[name] = v
	}
	own := c.ownUsage()
	m["skill"] = Verb{Run: c.skill, Args: "[--check]", Desc: "rewrite the manual from the verbs, in all three places it is read", Usage: own}
	m["version"] = Verb{Run: c.version, Desc: "print the version, to tell a release from a local build", Usage: own}
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

// group is one section of a manual: the prose a person wrote for it, and the
// verbs that share that prose, in the order the manual reads them.
type group struct {
	prose string
	paths []string
}

// groups are the manual's sections. Verbs that share a Usage share a section —
// build, wasm, check, run and workerd are all stage's — and each appears once.
func (c Command) groups() []group {
	verbs := c.all()
	var out []group
	at := map[string]int{}
	for _, name := range c.manualOrder(verbs) {
		u := verbs[name].Usage
		i, seen := at[u]
		if !seen {
			at[u] = len(out)
			out = append(out, group{prose: u})
			i = len(out) - 1
		}
		out[i].paths = append(out[i].paths, name)
	}
	return out
}

// index is what the binary prints with no verb: every section, flattened,
// because a terminal has no markdown renderer.
func (c Command) index() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: every verb. `%s <verb> --help` says what one takes.\n\n", c.Name, c.Name)
	for _, g := range c.groups() {
		b.WriteString(Flatten(c.section(g)))
		b.WriteString("\n")
	}
	return b.String()
}

// section is a group as the manual shows it: the prose a person wrote, then
// the verbs, rendered. The prose says why the group exists; nothing in it
// names a verb or a flag, because those are below it and generated.
func (c Command) section(g group) string {
	prose := strings.TrimRight(g.prose, "\n")
	verbs := Verbs(c.Name, c.all(), g.paths)
	if prose == "" {
		return verbs
	}
	return prose + "\n\n" + verbs
}

// render is the manual: Head, every section, Tail.
func (c Command) render() string {
	var b strings.Builder
	b.WriteString(withProvenance(c.Head, c.Name))
	for _, g := range c.groups() {
		b.WriteString(strings.TrimRight(c.section(g), "\n") + "\n\n")
	}
	b.WriteString(c.Tail)
	return b.String()
}

// withProvenance puts a line under a manual's frontmatter saying what wrote
// it and from what. A generated file that does not say it is generated is a
// file someone edits, and the edit is gone at the next build with nothing to
// say it ever happened.
func withProvenance(head, name string) string {
	note := "<!-- Generated by `" + name + " skill` from the verbs and the prose beside them.\n" +
		"     Never edit this file. Signatures come from each verb's Args and Flags,\n" +
		"     the line under one from its Desc, and the prose from a usage.md,\n" +
		"     head.md and tail.md. `go test` fails when this copy is stale. -->\n\n"
	lines := strings.SplitN(head, "---\n", 3)
	if len(lines) == 3 && strings.TrimSpace(lines[0]) == "" {
		return "---\n" + lines[1] + "---\n\n" + note + strings.TrimLeft(lines[2], "\n")
	}
	return note + head
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
	for name, body := range c.Skills {
		if err := c.writeSkill(stdout, name, body, bool(check)); err != nil {
			return err
		}
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

// writeSkill writes or checks one of c.Skills, in the three places a manual
// goes. It is the same work c.skill does for the command's own manual, on a
// body that was written rather than rendered.
func writeSkillTo(stdout io.Writer, paths []string, body string, check bool) error {
	for _, p := range paths {
		if check {
			have, err := os.ReadFile(p)
			if err != nil || string(have) != body {
				return fmt.Errorf("%s is stale; regenerate it with: dev skill", rel(p))
			}
			fmt.Fprintf(stdout, "%s is up to date\n", rel(p))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "wrote %s\n", rel(p))
	}
	return nil
}

// writeSkill resolves where a named skill goes, then writes or checks it.
func (c Command) writeSkill(stdout io.Writer, name, body string, check bool) error {
	root, err := root(".")
	if err != nil {
		return err
	}
	return writeSkillTo(stdout, skillPaths(root, name), body, check)
}

// skillPaths are the three copies of a manual for name: the one the release
// ships, and the ones each agent reads in this repo.
func skillPaths(root, name string) []string {
	return []string{
		filepath.Join(root, ShippedDir, name, SkillFile),
		filepath.Join(root, ClaudeDir, name, SkillFile),
		filepath.Join(root, AgentsDir, name, SkillFile),
	}
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
	dir, err := root(".")
	if err != nil {
		return
	}
	for name, body := range c.Skills {
		for _, p := range skillPaths(dir, name) {
			have, err := os.ReadFile(p)
			if err != nil || string(have) != body {
				t.Errorf("%s is stale; regenerate it with: %s skill", rel(p), c.Name)
			}
		}
	}
}
