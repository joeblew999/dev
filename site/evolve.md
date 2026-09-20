# The brief an agent gets when mise hands it the loop

`mise run site:evolve:agent` runs this. You are Claude Code, started headless
in this repo by a mise task, and this file is the whole of what you were
told.

## Where you are in the loop

There are two loops and they meet here.

`mise run site:evolve` is the mechanical one: it checks the deployed site,
writes the files that resolve what was found, deploys, and checks again. It
closes every finding dev already knows how to close — a finding carries
`FixedBy`, the writer whose artifact fixes it, and the writer runs.

**You start where that stops.** What survives `site:evolve` is, by
construction, what no existing writer claims. Each one is a question about
dev itself:

- a **writer** that should exist and does not, in `internal/seo/write.go`
- a **validator** that could have caught it before the deploy, in
  `internal/seo/validate.go` — a finding a checker made against a live URL
  that a file on disk already showed is a validator that should have said so
- a **`Fixes` prefix** that is wrong, so a finding nobody claims is really a
  finding routed nowhere
- the **site generator**, `site/gen/main.go`, when the fault is in this
  repo's own pages and not in the tool

Deciding which of those it is, is the work. Say which one you chose and why.

## Read the history before you decide

`.reports/site/` is every recorded run, and `mise run site:check` ends by
saying what moved:

    FIXED  kitsune      security.csp.missing
    NEW    scry         security/csp-unsafe — ...
    STOOD  seo-audit    TITLE_LENGTH — unchanged across 5 runs

That last line is the one to read first, and it is why this loop is not the
same loop every time.

- **STOOD at 2** is a finding waiting its turn. Take the obvious fix.
- **STOOD at 5 or more** has had the obvious fix tried four times. It did not
  work, or it was never the fix. Do not try it again — go and find out why
  the previous attempts failed, and if the honest answer is that no writer
  can close it, say that and close the loop on it instead.
- **NEW, right after a run of this task, is a regression you caused.** That
  is the most important line on the page. Fix it before anything else, or
  revert what caused it.

Read the last few files in `.reports/site/` directly if the summary is not
enough — they are the full reports, and the drift between two of them is
what your last change actually did.

## What you must not do

- **Do not commit.** The tree was clean when you started — the task refuses to
  run otherwise — so everything you do is recoverable with `git checkout .`,
  and that is the only safety net there is. Do not take it away.
- **Do not deploy a red tree.** `mise run check` green first, every time.
- **Do not put anything project-specific in the tool.** A favicon writer is
  something every repo gets; this repo's particular title is not. AGENTS.md
  is the rule and it is not negotiable.
- **Do not widen the job.** One finding. The loop runs again.

## The manual you were briefed with

`.claude/skills/dev/SKILL.md` is what Claude Code loaded when mise started
you, and it is generated from the verbs — so it describes the code as it was
when the task began. The moment you change a verb's flags, arguments or
description, the manual in your context is out of date and you are the only
one who knows it.

You do not need to regenerate it: `depends_post` runs `build` after you exit,
which rewrites all three copies, and `skill:check` fails if any is behind.
What you do need is to stop trusting your own briefing about anything you
have changed. Read the code, not the manual you were given.

Claude Code reads skills at startup and does not watch them. The next run of
this task is a fresh process and gets the new manual; a session already open
does not, which is why `session:check` runs afterwards and names them.

## What good looks like

One finding closed, or one honestly reported as not closable and why —
`perf.dom_size.metrics` on a page that is a whole manual may simply be true,
and inventing a writer for it would be worse than leaving it.

Then: `mise run check` green, `mise run lint` green, `mise run dup` and
`mise run dead` showing nothing new, and `mise run site:evolve` run for real
with the count moved. Report the before and after.

## Read first

`AGENTS.md`, then `internal/seo/usage.md`, then the registry in
`internal/seo/write.go`. The prose in this tree explains why things are as
they are, and the reason a thing is the way it is usually is the answer.
