---
name: dev
description: Build, check, run, release and deploy the commands of a repo on the mise + fnox + hk + packslip stack. Use before running go, npm, wrangler, fly, fnox or goreleaser by hand in such a repo: a mise task names a stage of a command directory and dev does the rest. Tests and releases run the same locally and in GitHub Actions, from the same tasks.
---

# dev

A repo on this stack is a few commands, each its own directory and Go module:
main.go, and beside it a worker.go and wrangler.toml if it deploys to
Cloudflare, a fly.toml if it deploys to Fly, a package.json and .gsx sources if
it has a UI. A repo that is one command keeps it at the root and uses `.`.
A new repo starts from `github.com/joeblew999/hello-stack`, which is this
stack and nothing else: clone it, rename its command, and its own CI proves
it still works.
Every command has the same stages, and a mise task names one:
`<cmd>:<stage>[:variant]`, so `mise run proxy:deploy` runs
`dev deploy cmd/proxy`. Run stages through their tasks (`mise tasks`
lists them), never by hand, and never call go, npm, wrangler, fly or fnox
directly when a task exists. Anything typed after a task name passes to the
command.

## Verbs

`<cmd> <verb> --help` says what one verb takes, and `<cmd> <verb> <sub> --help`
what one subcommand takes: its flags and what each one
means, read from the flags the verb registers, so they cannot drift from the
code. Here, `mise run help <verb>`.


The directory a verb acts on comes first; flags may follow anywhere, and
everything after a bare `--` goes to the program being run.

### Stages

- `dev build DIR`
  npm ci when stale, vite build, gsx generate, go build to `.bin/<dir>`, its
  skill if it is a `cli.Command`
- `dev wasm DIR [--env NAME]`
  the Worker's wasm for the environment (`build/tinygo` means TinyGo)
- `dev check DIR [--path P] [--expect TEXT]`
  gsx fmt, vet, test, the workerd round trip, the browser probe
- `dev run DIR [-- ARGS]`
  `.bin/<dir>` under fnox, replacing this process
- `dev workerd DIR [--env NAME]`
  the Worker on local workerd (wrangler dev)

### Deploying

- `dev url DIR [--deployed[=BOOL]] [--env NAME] [--local URL] [--refresh]`
  print the URL to talk to: the deployed app in DIR when `--deployed`, else
  `--local` (default empty). A Worker's needs the account's workers.dev
  subdomain: read once with the credentials in fnox, kept in gitignored
  `mise.local.toml`, `--refresh` asking again. A Fly app's is `<app>.fly.dev`.
- `dev deploy DIR [--env NAME] [--wait PATH] [-- FLAGS]`
  deploy what DIR holds. A Worker deploys from a throwaway copy of its
  `wrangler.toml`, so the ids wrangler writes back never reach git, and says
  what was created. A Fly app deploys with the repo root as build context,
  FLAGS going to flyctl, created first when the account lacks it (`FLY_ORG`
  names the org). With `--wait`, wait until it answers 200 at PATH
- `dev logs DIR [--env NAME]`
  stream the deployed app's logs (wrangler tail, flyctl logs)
- `dev smoke DIR [--env NAME] [--path P] [--expect TEXT] [--timeout DURATION]`
  run a Worker on local workerd with wrangler dev, request P (default `/`),
  and fail unless it answers 200 with TEXT in the body
- `dev wait URL [--timeout DURATION]`
  wait until URL answers 200 steadily
- `dev delete DIR [--env NAME] [--name APP] [--yes]`
  remove the deployed app in DIR, or APP (one a rename or an old config left
  behind), and for a Worker the KV namespaces wrangler provisioned for it,
  titled `<worker>-<binding>`; a namespace made by hand stays. Says what will
  go and asks, unless `--yes`

Which cloud DIR deploys to is read from it: `wrangler.toml` means Cloudflare
Workers, `fly.toml` means Fly; `--env` is a wrangler environment.
`DEPLOY_SUFFIX` in gitignored `mise.local.toml` gives a developer their own
copy of every app. Run from the repo root; needs fnox, and wrangler or flyctl.

### Secrets

- `dev secrets set DIR NAME|OWNER [--names LIST] [--generate] [--if-missing] [--env NAME]`
  store a secret in fnox and push it to the app in DIR; `--generate` makes a
  random value instead of prompting, `--if-missing` leaves an existing one
  alone. With `--names`, the project's `NAME<TAB>OWNER` lines, an owner such as
  a provider name resolves to its secret
- `dev secrets ci NAME...`
  give the repo's GitHub Actions each named secret from fnox (gh secret set,
  the value on stdin), for what CI must do with a credential: sign a
  release with the shared key, deploy to Fly as upstream's workflow does
- `dev secrets push DIR [--env NAME] [--fix TEMPLATE]`
  read `NAME<TAB>OWNER` lines on stdin and push each secret from fnox to the
  app in DIR; a missing one prints TEMPLATE with `{provider}` filled in, and
  any problem makes the exit code 1

### Releasing

- `dev release DIR [VERSION] [--snapshot] [--name NAME]`
  publish a GitHub Release of the command in DIR, the same locally and in
  GitHub Actions: VERSION here (vX.Y.Z), the pushed tag there. Build every
  platform with goreleaser, sign the packslip manifest, upload. NAME is the
  binary's name; default the repo's. Every directory under `skills/` ships as
  a skill. `--snapshot` builds, signs with a throwaway key and verifies,
  publishing nothing; check runs it.
- `dev release DIR --keygen [--rotate]`
  make the signing key every release is signed with: the private half into
  fnox (`PACKSLIP_SIGNING_KEY`) and the repo's Actions secrets, the public
  half into `packslip.pub` for consumers to pin as their `pubkey`. With
  `--rotate`, replace a key that already exists and say what every consumer
  must do; without it, an existing key is left alone.

Needs goreleaser, packslip and gh, and a clean tree to publish.

### Dependencies

- `dev deps list`
  list available Go module upgrades in every module, changing nothing
- `dev deps upgrade`
  interactively upgrade Go modules in every module

### The pinned session

- `dev session sync`
  write `.claude/skills` and the `.claude/settings.json` keys `session.toml`
  implies
- `dev session check`
  fail when either has drifted from `session.toml`
- `dev session verify [--update]`
  hold a fresh Claude Code session against `SESSION.lock`; `--update` records it
- `dev session bump [source]`
  move a pin in `session.toml` to upstream HEAD
- `dev session mcp`
  every MCP server `.mcp.json` declares connects

### The tool itself

- `dev skill [--check]`
  write `skills/dev/SKILL.md`, `.claude/skills/dev/SKILL.md` and `.agents/skills/dev/SKILL.md`
  from the verbs' own usage; `--check` fails when any is stale
- `dev version`
  print the version

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

## Working on a repo on this stack

These hold in any repo that pins dev, and ride this skill rather than each
repo's own AGENTS.md, so that fixing one fixes them everywhere.

- **Explain things in easy to understand ways.** A developer reads what you
  write; say it plainly, and say what a thing is before you say what to do
  about it.
- **Keep the code and its usage right.** A verb's `usage.md` is what a person
  and an agent both read to know what the verb does. Change a flag, an
  argument or a behaviour and change its usage in the same edit. `go test`
  holds the manual to the verbs and `cli.CheckUsage` holds that markdown to
  its shape, but nothing can check that the words are *true* — that is the
  author's job. Before calling a verb done, read its usage against its code:
  every flag the code registers appears, and every flag the usage names
  exists.
- **Every workflow is a mise task.** `mise tasks` lists them; mise is for
  orchestration over the code. `mise run test` before committing, and never
  call go, npm, wrangler, fly, fnox or goreleaser by hand when a task exists.
- **The manual is generated.** `<cmd> skill` renders it from the verbs; never
  edit a `SKILL.md` by hand, and `mise run check` fails when one is stale.
- **A package is one thing, named as the tasks name it**, and every verb has
  the one shape in `cli`: `Run(verb, args, stdout, stderr)`.
- **Comments say why.** The reason is what stops the same mistake twice; what
  the code does is already on the screen.
- **Test for real before saying done.** A path that could not be exercised —
  a cloud deploy, a machine you do not have — is said so plainly rather than
  assumed.
- **Commit only files you name.** Never `git add -A`.
- **Plans go in `.plans`** with a date-time stamp, steps checked off as each
  is done so any agent can pick the work up, and a DOD. Finished plans move
  to `.plans/done/`.
- **Raise issues as you find them.** Keep working to finish the task, but
  write the follow-up into the plan and tell the developer, so nothing is
  quietly dropped.
