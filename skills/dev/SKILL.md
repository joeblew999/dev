---
name: dev
description: Build, check, run and deploy the commands of a repo on the mise + fnox + hk + packslip stack. Use before running go, npm, wrangler, fly, fnox or goreleaser by hand in such a repo: a mise task names a stage of a command directory and dev does the rest.
---

# dev

A repo on this stack is a few commands, each its own directory and Go module:
main.go, and beside it a worker.go and wrangler.toml if it deploys to
Cloudflare, a fly.toml if it deploys to Fly, a package.json and .gsx sources if
it has a UI. A repo that is one command keeps it at the root and uses `.`.
A new repo gets the whole stack from `dev init`.
Every command has the same stages, and a mise task names one:
`<cmd>:<stage>[:variant]`, so `mise run proxy:deploy` runs
`dev deploy cmd/proxy`. Run stages through their tasks (`mise tasks`
lists them), never by hand, and never call go, npm, wrangler, fly or fnox
directly when a task exists. Anything typed after a task name passes to the
command.

## Verbs

The directory a verb acts on comes first; flags may follow anywhere, and
everything after a bare `--` goes to the program being run.

```
dev build DIR                             npm ci when stale, vite build, gsx generate, go build to bin/<dir>
dev wasm DIR [--env NAME]                 the Worker's wasm for the environment (build/tinygo means TinyGo)
dev check DIR [--path P] [--expect TEXT]  gsx fmt, vet, test, the workerd round trip, the browser probe
dev run DIR [-- ARGS]                     bin/<dir> under fnox, replacing this process
dev workerd DIR [--env NAME]              the Worker on local workerd (wrangler dev)
```

```
dev url DIR [--deployed[=BOOL]] [--env NAME] [--local URL] [--refresh]
    print the URL to talk to: the deployed app in DIR when --deployed, else
    --local (default empty). A Worker's needs the account's workers.dev
    subdomain: read once with the credentials in fnox, kept in gitignored
    mise.local.toml, --refresh asking again. A Fly app's is <app>.fly.dev.
dev deploy DIR [--env NAME] [--wait PATH] [-- FLAGS]
    deploy what DIR holds. A Worker deploys from a throwaway copy of its
    wrangler.toml, so the ids wrangler writes back never reach git, and says
    what was created. A Fly app deploys with the repo root as build context,
    FLAGS going to flyctl. With --wait, wait until it answers 200 at PATH
dev logs DIR [--env NAME]
    stream the deployed app's logs (wrangler tail, flyctl logs)
dev smoke DIR [--env NAME] [--path P] [--expect TEXT] [--timeout DURATION]
    run a Worker on local workerd with wrangler dev, request P (default /),
    and fail unless it answers 200 with TEXT in the body
dev wait URL [--timeout DURATION]
    wait until URL answers 200 steadily
dev delete DIR [--env NAME] [--name APP] [--yes]
    remove the deployed app in DIR, or APP (one a rename or an old config left
    behind), and for a Worker the KV namespaces wrangler provisioned for it,
    titled <worker>-<binding>; a namespace made by hand stays. Says what will
    go and asks, unless --yes

Which cloud DIR deploys to is read from it: wrangler.toml means Cloudflare
Workers, fly.toml means Fly; --env is a wrangler environment. DEPLOY_SUFFIX
in gitignored mise.local.toml gives a developer their own copy of every app.
Run from the repo root; needs fnox, and wrangler or flyctl.
```

```
dev deps list      list available Go module upgrades in every module, changing nothing
dev deps upgrade   interactively upgrade Go modules in every module
```

```
dev init [DIR] [--name NAME] [--pin VERSION]
    write the stack into DIR (default .): mise.toml with the tools pinned and
    the stack's tasks, hk.pkl, session.toml, .mcp.json, the Claude Code
    settings and skill hook, the two workflows, .gitignore, AGENTS.md, and a
    first command cmd/NAME (an HTTP server answering /health) with its module
    and go.work. NAME defaults to DIR's name; the module path comes from the
    git remote, or example.com without one. VERSION is the dev release to
    pin; default this binary's own. Existing files are left alone and named.
    Then: mise trust && mise install && mise run test
```

```
dev release DIR [VERSION] [--snapshot] [--name NAME]
    publish a GitHub Release of the command in DIR: tag VERSION (vX.Y.Z; in CI
    the pushed tag), build every platform with goreleaser, sign the packslip
    manifest, upload. --snapshot builds, signs with a throwaway key and
    verifies, publishing nothing. NAME is the binary's name; default the
    repo's. Every directory under skills/ ships as a skill.

Needs goreleaser, packslip and gh, and a clean tree to publish.
```

```
dev secrets set DIR NAME|OWNER [--names LIST] [--generate] [--if-missing] [--env NAME]
    store a secret in fnox and push it to the app in DIR; --generate makes a
    random value instead of prompting, --if-missing leaves an existing one
    alone. With --names, the project's "NAME<TAB>OWNER" lines, an owner such as
    a provider name resolves to its secret
dev secrets push DIR [--env NAME] [--fix TEMPLATE]
    read "NAME<TAB>OWNER" lines on stdin and push each secret from fnox to the
    app in DIR; a missing one prints TEMPLATE with {provider} filled in, and
    any problem makes the exit code 1
```

```
dev session sync             write .claude/skills and the .claude/settings.json keys session.toml implies
dev session check            fail when either has drifted from session.toml
dev session verify [--update]  hold a fresh Claude Code session against SESSION.lock; --update records it
dev session bump [source]    move a pin in session.toml to upstream HEAD
dev session mcp              every MCP server .mcp.json declares connects
```

## What a repo supplies

- `[vars] worker` in mise.toml: the command whose secrets `secrets:*` manage.
- A `check` task, what `mise run test` runs after the stack's own checks.
- A `validate` task, what `deploy` runs first.
- A `secrets:list` task printing NAME<TAB>OWNER lines, what `secrets:*` work from.

## Rules the tool keeps

- Nothing personal in a committed file. Cloud credentials come from fnox; a
  Worker's provisioned ids never reach git (deploy runs on a throwaway copy of
  wrangler.toml); the account's workers.dev subdomain and a developer's
  DEPLOY_SUFFIX live in gitignored mise.local.toml.
- Secret values only ever pass through fnox and the deploy CLI, never an argument.
- Every error names its fix.
