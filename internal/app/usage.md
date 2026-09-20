### Deploying

These act on a deployed app. Which cloud that is comes from the directory
itself — a wrangler.toml means Cloudflare Workers, a fly.toml means Fly — so
you never tell the tool which one you are on, and the same task works for
either.

Only a Worker can be run locally, so the verbs that do that take flags a Fly
app has no use for. `--help` cannot tell you which of them your directory
takes: it is printed before any directory is read, so it lists both clouds'
flags together. What it does instead is refuse the ones your cloud cannot act
on, at the moment you pass one, and say why.

`url DIR` prints the address to talk to. With `--local URL` that is the one
you gave; otherwise it is the deployed address, because that is the only
other address there is.

`health DIR` asks that address and says what came back: the status, how long
it took, and the response headers — `Content-Type`, `Cache-Control`, the
CSP, HSTS, and the three a checker faults a site for missing. A header is
the half of a response that decides how a browser and a crawler treat
everything else, and it is the half nobody sees without asking. Running it
against a directory on each cloud is how you find out whether two deploys of
one thing really are serving it the same way, which until this verb existed
meant curl and comparing two scrollbacks by eye. Nothing about it is
per-cloud: it reads the same deployed address `url` prints, so a cloud added
later gets it by declaring where its apps live and nothing more.

`delete DIR` takes it down, and answering "there is nothing to delete" is a
success rather than an error — a deploy you cannot remove is one you will
hesitate to make.

A developer's own copy of every app comes from DEPLOY_SUFFIX in gitignored
mise.local.toml, so two people deploying the same repo never fight over one.

`domains` is every domain on the Cloudflare account and what each points at.
A domain that answers nothing is easy to end up with and hard to notice: it
was registered for a project that did not happen, or moved, or was bought
defensively beside one in use — and the bill and the dashboard look the same
either way. It also answers the question that comes before any experiment
with fronting: which of these can be used without touching anything that
matters.

`front HOST ORIGIN` puts Cloudflare in front of an app, and `unfront HOST`
takes it away. They say what would change and change nothing until `--apply`,
and the zone is named with `--zone` rather than worked out from the hostname —
a token here reaches every zone on an account, and this changes how Cloudflare
serves every site on the one it touches.

They drive opentofu with Cloudflare's own provider, and dev writes the
configuration so nobody here writes Terraform by hand. Both are binaries the
registry fetches, which is the point: the Cloudflare surface a project wants
is vast — Access, WAF, load balancers, certificates — and forty hand-rolled
lines per resource is a worse trade than a provider Cloudflare generates and
tests. What comes with it is a plan that says exactly what would happen, state
that records what dev made so removing it is exact, and a provider that knows
a zone setting cannot be destroyed, only set to something else.

Reading stays dev's own, because a read wants no state file, no plan and no
provider: it wants an answer, and it has one in about a second.

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

On a directory that already deploys somewhere, `--to` is checked rather than
ignored: naming the cloud it is already on changes nothing, and naming a
different one is refused. It used to be read only when there was no config, so
`--to cloudflare` on a Fly directory deployed to Fly without a word, and a
typo did the same.
