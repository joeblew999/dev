### Stages

- build
  npm ci when stale, vite build, gsx generate, go build to `.bin/<dir>`, its
  skill if it is a `cli.Command`
- wasm
  the Worker's wasm for the environment (`build/tinygo` means TinyGo)
- check
  gsx fmt, vet, test, the workerd round trip, the browser probe
- run
  `.bin/<dir>` under fnox, replacing this process
- workerd
  the Worker on local workerd (wrangler dev)
