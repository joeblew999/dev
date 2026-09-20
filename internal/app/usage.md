### Deploying

These act on a deployed app. Which cloud that is comes from the directory
itself — a wrangler.toml means Cloudflare Workers, a fly.toml means Fly — so
you never tell the tool which one you are on, and the same task works for
either.

Only a Worker can be run locally, so the verbs that do that take flags a Fly
app has no use for. Asking one of these verbs for help resolves the directory
first, so it tells you what your directory really takes.

A developer's own copy of every app comes from DEPLOY_SUFFIX in gitignored
mise.local.toml, so two people deploying the same repo never fight over one.

A directory deploys to the cloud its config names, and one with no config
deploys nowhere. `deploy DIR --to fly` or `--to cloudflare` writes that
cloud's conventional config and carries on, the way a release writes
goreleaser's when a repo has none. This one stays and is committed: the file's
presence is what names the target, so a temporary one would deploy nowhere the
next time anybody looked. Read it — it is the convention, not a ceiling.
