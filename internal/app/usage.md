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

`list DIR` says what that directory's cloud has deployed, marking the one the
directory is — because "what did I leave running" is the question a deploy
raises and nothing here could answer. It is the other half of being able to
remove something: a suffixed copy is easy to make and easy to forget.

`logs DIR` streams, which is for a person watching a deploy. `logs DIR --json`
asks the bounded question instead — the last `--limit` events over the last
`--since` — so a script, a report or an agent can read it, since a stream has
no end any of those can wait for.

How far back that reaches is not the same on both, and the note on stderr says
which. Cloudflare stores Workers Logs for seven days and answers a query over
them, for a Worker whose config enables observability — which the config
written here does. Fly streams from its machines, so what comes back is what
`flyctl` still holds in its buffer: recent, and not a window. Fly does keep
seven days behind an HTTP API, and dev does not use it, because Fly's own
documentation calls that API not officially documented for external use.


A directory deploys to the cloud its config names, and one with no config
deploys nowhere. `deploy DIR --to fly` or `--to cloudflare` writes that
cloud's conventional config and carries on, the way a release writes
goreleaser's when a repo has none. This one stays and is committed: the file's
presence is what names the target, so a temporary one would deploy nowhere the
next time anybody looked. Read it — it is the convention, not a ceiling.
