---
name: cli
description: Write a command on this stack's verb system, github.com/joeblew999/dev/cli. Use when adding a verb, a flag or a subcommand to a command, when writing a new command, or when its manual, its --help or its usage.md is wrong. The API, the shapes a verb takes, and the two test helpers that hold a manual to its code.
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
    Verbs:   map[string]cli.Verb{"serve": {Run: serve, Usage: usage}},
    Head:    head,
    Tail:    tail,
}

func main() { cli.Main(app) }
```

- `Name` — the binary's name. Its manual is `skills/<Name>/SKILL.md`.
- `Verbs` — verb name to `Verb{Run, Usage}`. Verbs that share a package share
  one `Usage`, and the manual prints it once.
- `Default` — the verb run when none is given. A server uses this, so
  `hello` alone serves. Empty means the index is printed instead.
- `Head`, `Tail` — the manual's prose before and after the verbs.
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
  as the sentence a person needs.
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

What a person writes is markdown beside the code: `usage.md` per package for
its verbs, `head.md` and `tail.md` for the prose around them, embedded with
`//go:embed`. Files rather than Go string constants, because a Go raw string
is backtick-delimited and so cannot hold inline code.

Name that markdown in the build task's `sources`, and the three manual copies
in its `outputs`. mise watches `.go` by default, so a prose-only edit
otherwise leaves the binary stale while reporting it fresh.

## The two tests

```go
func TestSkill(t *testing.T) { cli.CheckSkill(t, app) }
func TestUsage(t *testing.T) { cli.CheckUsage(t, app) }
```

- `CheckSkill` fails when any copy of the manual differs from what the verbs
  render. It recompiles, so it is the one guard a stale binary cannot fool.
- `CheckUsage` fails when a verb's usage uses markdown the terminal rendering
  cannot read, or leaves a `<placeholder>` outside inline code.

## The markdown a usage.md may use

Headings, `- ` list items with two-space continuations, paragraphs and inline
code. A verb is one list item: the signature its first line, the description
its continuation. That is the subset the terminal rendering reads — the same
text has to work with no renderer at all.

`head.md` and `tail.md` only ever reach the manual, never a terminal, so they
may use all of markdown. Two rules hold everywhere:

- Write every `<placeholder>` in backticks. Unfenced, a markdown renderer
  takes `<app>` or `NAME<TAB>OWNER` for an HTML tag and the reader never sees
  it; in inline code it escapes correctly.
- Emphasis is banned, so `*` and `_` stay literal. Usage text says things like
  `**/*.go` and `secrets:*`, and a flattener that unwound emphasis would turn
  the first into `/*.go`.

## Porting a command that predates this

Usage that is plain text rather than markdown is fenced in the manual, exactly
as every usage was, and the terminal is unchanged — so port when you choose, a
package at a time. Move the `Usage` const into a `usage.md` beside it,
`//go:embed usage.md`, add `cli.CheckUsage` to the test, and name the markdown
in `sources`.
