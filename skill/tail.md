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
