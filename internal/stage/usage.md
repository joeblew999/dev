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
