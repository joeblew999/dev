### The pinned session

A repo's Claude Code session is a fact about the repo, not about the machine
it is read on: which skills an agent may load, and which of the settings keys
those skills need. `session.toml` is where that is written, and these verbs
are the only things that act on it — one file decides, and every developer
and every agent on the repo gets the same session from it.

`sync` writes what the file implies and `check` refuses when what is on disk
has drifted from it, so the pair is the usual gate: run the first, let CI run
the second. `verify` goes further and holds a real Claude Code session against
`SESSION.lock`, because a skill that is on disk is not yet a skill the agent
actually loaded; `--update` records what it saw instead of only reporting it.
`bump` moves a pin to upstream HEAD when a skill should follow it, and `mcp`
says which MCP servers `.mcp.json` really connects.

Every error names the sync that would fix it, in this repo's own spelling:
`session.toml` sets `sync_command`, so the advice reads as the task you run
rather than as a binary you may never call directly.
