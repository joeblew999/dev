### The pinned session

- `dev session sync`
  write `.claude/skills` and the `.claude/settings.json` keys `session.toml`
  implies
- `dev session check`
  fail when either has drifted from `session.toml`
- `dev session verify [--update]`
  hold a fresh Claude Code session against `SESSION.lock`; `--update` records it
- `dev session bump [source]`
  move a pin in `session.toml` to upstream HEAD
- `dev session mcp`
  every MCP server `.mcp.json` declares connects
