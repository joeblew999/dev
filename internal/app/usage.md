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

`domains` is every domain on the Cloudflare account and what each points at.
A domain that answers nothing is easy to end up with and hard to notice: it
was registered for a project that did not happen, or moved, or was bought
defensively beside one in use — and the bill and the dashboard look the same
either way. It also answers the question that comes before any experiment
with fronting: which of these can be used without touching anything that
matters.

`fronting HOST` says what stands in front of a host and whether that
arrangement can work. Both read and change nothing, deliberately: a token
that can read a zone's settings can usually write them, and one that reaches
every zone would change how every site on a domain is served from a command
somebody ran about one app.

`list DIR` says what that directory's cloud has deployed, marking the one the
directory is — because "what did I leave running" is the question a deploy
raises and nothing here could answer. It is the other half of being able to
remove something: a suffixed copy is easy to make and easy to forget.

`logs DIR` streams, which is for a person watching a deploy. `logs DIR --json`
asks the bounded question instead — the last `--limit` events over the last
`--since` — so a script, a report or an agent can read it, since a stream has
no end any of those can wait for.

Every event says who wrote it: your application, or the platform running it.
Most of what comes back is never the application — Fly's image pulls and
firecracker lines, Cloudflare's own record of each invocation — and without
that distinction "my app logged nothing" and "my app's lines are buried in
machinery" look identical.

What both clouds really record about a request — the method, the URL, the
status and the id — is unified; the id is what lets you find every line about
the same request. The rest is not, because it is not the same on both:
Cloudflare keeps request headers, CPU time and wall time, and Fly's log schema
has no headers at all. `--raw` carries each cloud's own record alongside, so
what only one of them has is still reachable rather than lost to a shape that
had nowhere to put it.

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
