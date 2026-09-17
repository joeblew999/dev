### The pinned session

- session sync
  write `.claude/skills` and the `.claude/settings.json` keys `session.toml`
  implies
- session check
  fail when either has drifted from `session.toml`
- session verify
  hold a fresh Claude Code session against `SESSION.lock`; `--update` records it
- session bump
  move a pin in `session.toml` to upstream HEAD
- session mcp
  every MCP server `.mcp.json` declares connects
