---
name: cli
description: Write a command on this stack's verb system, github.com/joeblew999/dev/cli. Use when adding a verb, a flag or a subcommand to a command, when writing a new command, or when its manual, its --help or its usage.md is wrong. The API, what a verb declares, and the test helpers that hold a manual to its code.
---

# cli

`github.com/joeblew999/dev/cli` is the verb system every command on this stack
shares. A command declares its verbs and its prose; `cli` runs it, prints what
a terminal needs, and renders its manual from the same declaration. The manual
therefore cannot drift from the binary — nothing copies anything.

Write a command against this rather than `flag` directly, and it gets
`<cmd> skill`, `<cmd> version` and `--help` at every level for free.

## The shape

```go
var app = cli.Command{
    Name:    "hello",
    Default: "serve",
    Verbs: map[string]cli.Verb{
        "serve": {Run: serve, Args: "DIR", Flags: serveFlags,
                  Desc: "answer /health on the address", Usage: usage},
    },
    Head: head,
    Tail: tail,
}

func main() { cli.Main(app) }
```

A verb declares what only it knows and nothing that is written elsewhere:

- `Args` — its positionals, `"DIR"` or `"URL"` or `"DIR [VERSION]"`.
- `Flags` — a func that registers its flags. `cli` renders the signature from
  it and `Run` calls the same func, so there is one registration.
- `Desc` — one line saying what this verb is for, printed under its
  signature. Never the signature itself: that is rendered.
- `Usage` — the prose for the group this verb belongs to: why these verbs
  exist and what they share. Verbs that share it share a section.
- `Subs` — its subcommands, each with its own `Args` and `Flags`.

So `dev check DIR [--expect TEXT] [--path P]` is never typed, and neither is
the line under it. A flag cannot be missing from a manual, because the manual
is made from the flags.

- `Name` — the binary's name. Its manual is `skills/<Name>/SKILL.md`.
- `Verbs` — verb name to `Verb`. Verbs that share a package share one
  `Usage`, which is that section's prose, and the manual prints it once.
- `Default` — the verb run when none is given. A server uses this, so
  `hello` alone serves. Empty means the index is printed instead.
- `Skill` — the manual a person writes: frontmatter, prose, and a `VerbMarker`
  line where the rendered verbs belong. One document, because it is one
  document; with no marker they go last.
- `Order` — the manual's reading order, verb names, each standing for the
  group that shares its usage. Empty means verb-name order.
- `Version` — what `<cmd> version` prints; a release sets it through
  `-X main.version`.
- `Skills` — manuals this command ships that are not its own verbs, name to
  body, written to `skills/<name>/` beside its own.

## A verb

```go
func serve(verb string, args []string, stdout, stderr io.Writer) error {
    fs := cli.Flags(verb, stderr)
    addr := fs.String("addr", "127.0.0.1:8080", "address to listen on")
    rest, err := cli.ParseInterleaved(fs, args)
    if err != nil {
        return cli.Usagef("%s: %v", verb, err)
    }
    ...
}
```

Every verb is a `cli.Runner`: `func(verb string, args []string, stdout, stderr
io.Writer) error`. The verb it was called as comes first, so one function can
answer to several.

- `cli.Flags(name, stderr)` — a `*flag.FlagSet` that reports to stderr and
  never exits. Each flag's third argument is what `--help` prints, so write it
  as the sentence a person needs, and backquote the word that names its
  value: `"wrangler environment `+"`NAME`"+`"` prints as `[--env NAME]` in the
  manual and `-env NAME` under `--help`, from one string.
- `cli.Value(fs, name)` and `cli.Given(fs, name)` — read a flag back by name,
  which is what registering through a func costs at the call site.
- `cli.ParseInterleaved(fs, args)` — flags wherever they appear, positionals
  returned, everything after a bare `--` passed through verbatim. mise appends
  what a developer typed after a task name, so flags cannot be required first.
- `cli.DirAnd(fs, args, n)` — for a verb that acts on a directory:
  `DIR [flags and positionals in any order]`, with `n` positionals after it,
  or `-1` for any number.
- `cli.Bool` — a bool flag that also accepts `""` as false, so a task can pass
  `--flag=$var` with the variable unset.
- `cli.Usagef(...)` — the arguments were wrong. The command prints the verb's
  usage after it and exits 2. Pass the underlying error as an argument and a
  request for help is recognised rather than reported as a failure.
- `cli.Confirm(stdin, out, prompt)` — ask before something destructive. No
  terminal means no.
- `cli.HelpRequested(args)` — for a verb that dispatches on a subcommand, or
  wants `DIR` first: ask this before enforcing either, or `--help` is answered
  with a complaint about what is missing.

## Its manual

`<cmd> skill` writes three copies of the manual — `skills/<name>/`, which the
release ships, and `.claude/skills/<name>/` and `.agents/skills/<name>/`,
which that repo's own agents read. `dev build` runs it after every build.
Never edit a `SKILL.md`; it is written from the verbs.

What a person writes is markdown beside the code it describes, which is the
whole rule: `usage.md` in each package for its verbs, `skill.md` beside the
command's own main.go for the manual around them, and a library's own manual
in that library's directory. Embedded with `//go:embed`. Files rather than Go string constants, because a Go raw string
is backtick-delimited and so cannot hold inline code.

A `usage.md` holds no verbs and no flags — those are rendered under it. It
holds only what no verb can say: why this group exists, what they share, what
a repo has to supply. A test fails when it starts listing again.

Name that markdown in the build task's `sources`, and the three manual copies
in its `outputs`. mise watches `.go` by default, so a prose-only edit
otherwise leaves the binary stale while reporting it fresh.

## How a manual reaches another repo

Three routes, and which one a skill takes decides whether it is committed.

- **The repo's own commands.** `<cmd> skill` writes the three copies and they
  are committed, so the repo's own sessions read its own manual with no
  release and no pin. A changed verb reaches that repo's agent on the next
  build.
- **A tool it pins.** `dev release` turns every `skills/<name>/` directory
  into a packslip resource, so a command's manual ships with the binary. A
  repo pins the tool in `mise.toml` with the public key its releases are
  signed with, and mise's `[settings.skills] auto_sync` links the manual into
  `.claude/skills/<name>/`. Those links are gitignored: mise writes them per
  developer from the pinned versions, and `prune` removes what no pin names.
- **An upstream that ships no releases.** Vendored by commit through
  `session.toml` and committed, because there is no version to link from.

So a manual is committed when the repo produces it or vendors it, and
gitignored when a pin produces it. Committing a link, or ignoring your own
command's manual, is how a repo ends up with two of something.

A skill a command ships through `Skills` travels the second way: it is a
`skills/<name>/` directory like any other, so the release carries it and a
consumer gets it on a pin bump. That is how this manual reaches you.

## The two tests

```go
func TestSkill(t *testing.T)  { cli.CheckSkill(t, app) }
func TestUsage(t *testing.T)      { cli.CheckUsage(t, app) }
func TestDescribed(t *testing.T)  { cli.CheckDescribed(t, app) }
```

- `CheckSkill` fails when any copy of the manual differs from what the verbs
  render. It recompiles, so it is the one guard a stale binary cannot fool.
- `CheckUsage` fails when a verb's usage uses markdown the terminal rendering
  cannot read, or leaves a `<placeholder>` outside inline code.
- `CheckDescribed` fails when a verb has no `Desc`, and when a `usage.md`
  lists verbs instead of explaining them.

## Seeing what an agent can read

`<cmd> skills` lists both agent directories and says where each entry came
from: a real directory the repo wrote, or a symlink mise made from a tool the
repo pins. It reads the directories rather than asking an agent what it
loaded, because an agent is told what is available and cannot enumerate it —
the directory is the fact.

It also says when a skill is in one directory and not the other, which is a
skill that agent cannot see. mise syncs a pinned tool's skills into
`.claude/skills` and nowhere else, so that happens by default.

## Holding a skill to the code it documents

A verb's signature cannot drift — `cli` renders it from the flags. A skill
that explains a library is prose, and prose drifts. This one did, twice in a
day: it showed a field before the code had it and named a function after the
code lost it.

```go
func TestSkillNames(t *testing.T) {
    skillcheck.Names(t, mySkill, "internal/thing")
}
```

`skillcheck.Names` fails when the skill names a function, type or struct field
no listed package has. Telling an identifier from a placeholder is the whole
trick: a name needs a lowercase letter in it to count, so `Desc` is checked
and `DIR` is not, with no list of exceptions to keep.

It is `github.com/joeblew999/dev/cli/skillcheck`, a package of its own because
it parses Go and `go/parser` is 66 packages. `cli` is linked into every
command built on it, a Worker's wasm included, so what only a test needs
stays out of it.

## The markdown a usage.md may use

Headings, `- ` list items with two-space continuations, paragraphs and inline
code. A verb is one list item: the signature its first line, the description
its continuation. That is the subset the terminal rendering reads — the same
text has to work with no renderer at all.

A `skill.md` only ever reaches the manual, never a terminal, so it may use all
of markdown. Two rules hold everywhere:

- Write every `<placeholder>` in backticks. Unfenced, a markdown renderer
  takes `<app>` or `NAME<TAB>OWNER` for an HTML tag and the reader never sees
  it; in inline code it escapes correctly.
- Emphasis is banned, so `*` and `_` stay literal. Usage text says things like
  `**/*.go` and `secrets:*`, and a flattener that unwound emphasis would turn
  the first into `/*.go`.

## If your command stopped compiling

Breaking a consumer is allowed here: the tool says what is wrong and the repo
that broke fixes itself. So these are the errors this version produces and
what each one means.

- `unknown field Head` / `unknown field Tail` — a command's manual is one
  document now, not a prose-before and a prose-after. Join your `head.md` and
  `tail.md` into a `skill.md` with a `<!-- verbs -->` line where they met, and
  set `Skill:` instead of `Head:` and `Tail:`.
- `undefined: cli.Entry` — it read a verb's item out of a usage.md, and a
  usage.md has no items any more. Whatever called it wants `cli.Verbs` or
  nothing.
- A verb prints with no flags in its signature — give it `Flags`, the func
  that registers them, and `Args` for its positionals. It had them in prose
  before; now it declares them.
- A verb prints as a bare signature with nothing under it — give it `Desc`.
  `cli.CheckDescribed` fails on this, so add that test and it tells you which.

## Porting a command that predates this

Usage that is plain text rather than markdown is fenced in the manual, exactly
as every usage was, and the terminal is unchanged — so port when you choose, a
package at a time. Give each verb its `Args`, `Flags` and `Desc`, move the
prose around them into a `usage.md`, `//go:embed usage.md`, add
`cli.CheckUsage` and `cli.CheckDescribed` to the test, and name the markdown
in `sources`.
