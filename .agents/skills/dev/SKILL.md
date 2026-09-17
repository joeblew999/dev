---
name: dev
description: Build, check, run, release and deploy the commands of a repo on the mise + fnox + hk + packslip stack. Use before running go, npm, wrangler, fly, fnox or goreleaser by hand in such a repo: a mise task names a stage of a command directory and dev does the rest. Tests and releases run the same locally and in GitHub Actions, from the same tasks.
---

# dev

A repo on this stack is a few commands, each its own directory and Go module:
main.go, and beside it a worker.go and wrangler.toml if it deploys to
Cloudflare, a fly.toml if it deploys to Fly, a package.json and .gsx sources if
it has a UI. A repo that is one command keeps it at the root and uses `.`.
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

Writing a command, a verb or a flag is the `cli` skill's subject: the API, the
shapes a verb takes, the markdown a `usage.md` may use, and the two test
helpers. It ships beside this one.

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
