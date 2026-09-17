## What a repo supplies

- `[vars] worker` in mise.toml: the command whose secrets `secrets:*` manage.
- A `check` task, what `mise run test` runs after the stack's own checks.
- A `validate` task, what `deploy` runs first.
- A `secrets:list` task printing `NAME<TAB>OWNER` lines, what `secrets:*` work from.

## A command's manual

A command's manual is rendered from its verbs, so it cannot drift from the
binary: `<cmd> skill` writes it, `dev build` runs that, and `go test` fails
when a copy is stale. Never edit a `SKILL.md` by hand.

What a person writes is markdown beside the code: `usage.md` in each package
for its verbs, `head.md` and `tail.md` for the prose around them. They are
files rather than Go string constants because a Go raw string is
backtick-delimited, so it can never hold inline code.

A repo written before this keeps working: usage that is plain text rather than
markdown is fenced in the manual, exactly as every usage was, and the terminal
is unchanged. Port when you choose, a package at a time — move the `Usage`
const into a `usage.md` beside it, `//go:embed usage.md`, and add
`cli.CheckUsage` to the command's test. Two things go with the port: name the
markdown in the build task's `sources` (mise reads `.go` by default, so a
prose-only edit otherwise leaves the binary stale), and list the manual's
copies in `outputs`.

A command writes its manual from prose compiled into it, so what it writes is
only as current as the binary. `<cmd> skill` and `<cmd> skill --check` refuse
when anything they were built from has changed since, naming the file: from
inside a stale binary both would otherwise report success — one rewriting
every copy from old bytes, the other comparing an old render against equally
old files. Rebuild and run them again. Only `go test` is immune, because it
recompiles.

Keep `usage.md` to headings, `- ` list items with two-space continuations,
paragraphs and inline code. A verb is one list item: the signature its first
line, the description its continuation. That subset is what the terminal
rendering reads — the same text has to work with no renderer at all — and
`cli.CheckUsage` holds it there from a test. `head.md` and `tail.md` only ever
reach the manual, never a terminal, so they may use all of markdown. Two rules
earn their keep:

- Write every `<placeholder>` in backticks — in `usage.md`, and in `head.md`
  and `tail.md` too. Unfenced, a markdown renderer takes `<app>` or
  `NAME<TAB>OWNER` for an HTML tag and the reader never sees it; in inline
  code it escapes correctly. This one shipped in this file once, which is why
  the check covers prose and not only verbs.
- Emphasis is banned, so `*` and `_` stay literal. Usage text says things like
  `**/*.go` and `secrets:*`, and a flattener that unwound emphasis would turn
  the first into `/*.go`.

## Rules the tool keeps

- Nothing personal in a committed file. Cloud credentials come from fnox; a
  Worker's provisioned ids never reach git (deploy runs on a throwaway copy of
  wrangler.toml); the account's workers.dev subdomain and a developer's
  DEPLOY_SUFFIX live in gitignored mise.local.toml.
- Secret values only ever pass through fnox and the deploy CLI, never an argument.
- Every error names its fix.
- Every task runs the same locally and in GitHub Actions: mise run test and
  mise run release are what CI runs, from the one mise.toml. Local is the fast
  path day to day; CI proves a machine nobody set up. Neither replaces the other.
