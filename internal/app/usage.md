### Deploying

- url
  print the URL to talk to: the deployed app in DIR when `--deployed`, else
  `--local` (default empty). A Worker's needs the account's workers.dev
  subdomain: read once with the credentials in fnox, kept in gitignored
  `mise.local.toml`, `--refresh` asking again. A Fly app's is `<app>.fly.dev`.
- deploy
  deploy what DIR holds. A Worker deploys from a throwaway copy of its
  `wrangler.toml`, so the ids wrangler writes back never reach git, and says
  what was created. A Fly app deploys with the repo root as build context,
  FLAGS going to flyctl, created first when the account lacks it (`FLY_ORG`
  names the org). With `--wait`, wait until it answers 200 at that path
- logs
  stream the deployed app's logs (wrangler tail, flyctl logs)
- smoke
  run a Worker on local workerd with wrangler dev, request `--path`, and fail
  unless it answers 200 with `--expect` in the body. Only a Worker is run
  locally, so a Fly app's smoke takes none of these flags; the signature shows
  the Worker's, and `dev smoke DIR --help` shows what that directory's cloud
  really takes.
- wait
  wait until URL answers 200 steadily
- delete
  remove the deployed app in DIR, or `--name` (one a rename or an old config
  left behind), and for a Worker the KV namespaces wrangler provisioned for
  it, titled `<worker>-<binding>`; a namespace made by hand stays. Says what
  will go and asks, unless `--yes`

Which cloud DIR deploys to is read from it: `wrangler.toml` means Cloudflare
Workers, `fly.toml` means Fly; `--env` is a wrangler environment.
`DEPLOY_SUFFIX` in gitignored `mise.local.toml` gives a developer their own
copy of every app. Run from the repo root; needs fnox, and wrangler or flyctl.
