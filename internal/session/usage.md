### The pinned session

A repo's Claude Code session is a fact about the repo, not about the machine
it is read on: which skills an agent may load, and which of the settings keys
those skills need. `session.toml` is where that is written, and these verbs
are the only things that act on it — one file decides, and every developer
and every agent on the repo gets the same session from it.

Skills are written to every directory an agent reads — `.claude/skills`
for Claude Code and `.agents/skills` for Copilot — from one set. An agent
reads its own and no other, so a skill in one alone is one the rest cannot
see.

#### Where skills come from

`presets = ["recommended"]` takes a set dev keeps, at the commits this dev
pins. The repo decides to take them; dev decides what they are, and they move
when you upgrade dev and at no other time.

`recommended` is four skills, each of which has been synced and checked here:
`tdd` and `writing-for-agents` from mattpocock/skills, and
`systematic-debugging` and `brainstorming` from obra/superpowers. It is small
because a skill earns its place by being one this stack works the way of, not
by existing.

A `[source.<name>]` block is one GitHub repository at one commit and the
skills taken from it — for an upstream dev does not curate, or one that is
yours. `dir` is where that repository keeps them, `skills` unless it says
otherwise. A name in `skills` may be a path within it, because upstreams file
skills by category: `engineering/tdd` is taken from there and vendored as
`tdd`, which is where an agent looks.

A repository that ships as a Claude Code plugin says where its skills are in
`.claude-plugin/plugin.json`, and that list is used when the name alone does
not resolve — so `tdd` finds `./skills/engineering/tdd` without the repo
having to know the category. An upstream that ships no manifest, or ships one
that lists no skills, is not unusual and is not an error; convention is tried
first for exactly that reason.

`ref` is a full commit sha and nothing else. A branch or a tag is a name
upstream can move, so a repo pinned to one gets a different session on a
different day with the file unchanged. Two pins that would land on the same
name are refused rather than one quietly overwriting the other.

#### The verbs

`sync` writes what the file implies and `check` refuses when what is on disk
has drifted from it, so the pair is the usual gate: run the first, let CI run
the second. `verify` goes further and holds a real Claude Code session against
`SESSION.lock`, because a skill that is on disk is not yet a skill the agent
actually loaded; `--update` records what it saw instead of only reporting it.
`bump` moves a pin to upstream HEAD when a skill should follow it — only for
sources this repo declared, since a preset has no ref here to rewrite. `mcp`
says which MCP servers `.mcp.json` really connects.

`remove` is the undo: it takes back every skill sync put here, in every
directory, and leaves alone anything sync did not put there — a skill this
repo wrote itself, one mise linked from a pinned tool, one dropped in by hand.
`SKILLS.lock` is the list of what sync owns, and it is the only thing
consulted, so adopting a preset is a decision that can be reversed rather than
one that has to be lived with. Dropping a pin and syncing again does the same
for one source; deleting `session.toml` outright does not, and sync says so
and names this verb rather than advising you to fix a file you deleted.

There are two locks and they answer different questions. `SKILLS.lock` is
what sync wrote, so check can verify the files without downloading anything.
`SESSION.lock` is what a running Claude Code session reported it could load,
which only verify writes and only a real session can answer.

A session that is already open may be holding a stale view of skills that
just changed: an edited `SKILL.md` is documented to apply without a restart,
and a newly added one was seen to, but a removed one is documented neither
way. `sync` and `remove` both say so when they find such a session, and name
`/reload-plugins` — which reloads skills without losing the session — rather
than telling you to restart.

Every error names the sync that would fix it, in this repo's own spelling:
`session.toml` sets `sync_command`, so the advice reads as the task you run
rather than as a binary you may never call directly.
