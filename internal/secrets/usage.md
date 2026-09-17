### Secrets

A secret lives in fnox and nowhere else. These move it from there to wherever
it is needed — the deployed app, GitHub Actions — and its value never appears
as a command argument, so it cannot end up in a shell history or a log.

Which secrets an app needs is the repo's business, not the tool's: a repo
supplies a task that lists them, and these work from that list.
