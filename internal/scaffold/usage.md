### Starting a repo

- `dev init [DIR] [--name NAME] [--pin VERSION]`
  write the stack into DIR (default `.`): `mise.toml` with the tools pinned
  and the stack's tasks, `hk.pkl`, `session.toml`, `.mcp.json`, the Claude
  Code settings and skill hook, the two workflows, `.gitignore`, `AGENTS.md`,
  and a first command `cmd/NAME` (an HTTP server answering `/health`, a
  `cli.Command` whose skill its builds write) with its module, requiring the
  pinned dev, and `go.work`. NAME defaults to DIR's name; the module path
  comes from the git remote, or `example.com` without one. VERSION is the dev
  release to pin, with the public key its releases are signed with
  (`--pubkey`); default this binary's own; the other pins are the releases
  mise knows today. An existing file is left alone and named; a repo with a
  module at the root gets no workspace or nested module, and an existing
  command is kept. Then: `mise trust && mise install && mise run test`
