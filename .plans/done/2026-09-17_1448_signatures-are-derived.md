# The signature is derived; usage.md is description only

**Status:** DONE 2026-09-17 · **Created:** 2026-09-17
**Affects:** `cli.Verb`, every verb in this repo, every command in every repo
that builds on `cli`.

## What it was meant to be

`.plans/2026-09-17_0852_i18n-into-dev.md:231`, check I7:

> `dev skill --check` clean, and the rendered skill carries **every flag in the
> verb table** — the skill is rendered from the verbs, so it must not drift,
> and a dropped flag drops out of the manual silently.

That is the design. It was never implemented. `--rotate` is the proof: it was
registered in `release.go` and named in no manual, and stayed invisible until
it was found by reading both by hand.

## Why it is not true today

A signature is typed by hand in `usage.md`:

    - `dev check DIR [--path P] [--expect TEXT]`

and the same flags are registered in Go:

    fs.String("path", "/", "what the smoke check and the browser probe request")

Two copies, nothing comparing them. `usage.md` was meant to carry the extra
description only.

## What is derivable and what is not

| part of a signature | lives | derivable |
|---|---|---|
| the verb's name | the table | yes |
| its flags, types, defaults, help | the FlagSet | yes |
| its positionals (`DIR`, `URL`, `NAME\|OWNER`) | usage.md only | no |
| its subcommands (`secrets set`, `deps list`) | usage.md only | no |

So a verb must declare its positionals and its subcommands — neither is
written anywhere else, so neither is a second copy — and the flags derive.

## The shape

```go
type Verb struct {
    Run   Runner
    Args  string               // positionals: "DIR", "URL", "DIR [VERSION]"
    Flags func(*flag.FlagSet)  // registers them; Run calls it too
    Usage string               // the description, and nothing else
    Subs  map[string]Verb      // secrets set, deps list, session sync
}
```

`cli` renders `dev check DIR [--path P] [--expect TEXT]` from `Args` and
`Flags`. `usage.md` says only what the verb does.

## The exception, which the intent did not anticipate

`url`, `deploy`, `logs`, `smoke` and `delete` read DIR to learn which cloud
they are talking to, and the two clouds register different flags: fly's
`smoke` takes none where cloudflare's takes three, and `--env` means
"wrangler environment" for one and "not a Fly concept" for the other. A
manual written once cannot say that.

Those five declare the flags both clouds share. `<verb> DIR --help` resolves
the directory first and shows that cloud's truth. The manual says so.

## DOD

- [x] `Verb` carries `Args`, `Flags` and `Subs`; `Usage` is description only.
- [x] Every signature in the manual is rendered, none typed.
- [x] A test fails when a verb registers a flag the manual does not show —
      I7, implemented rather than described.
- [x] `--rotate` appears without anyone having typed it.
- [x] The five cloud verbs' exception is written where a reader meets it.
- [x] `mise run test` green; the cli skill says what a verb declares.

## Work

- [x] 1. `Verb.Args` + `Verb.Flags`; `cli` renders a signature from them.
      `Signature` walks a throwaway FlagSet the verb registers into, so
      nothing has to run. Placeholders come from the standard library's own
      convention — a backquoted word in a flag's help names its value — so
      one string feeds both the manual and `--help`.
- [x] 2. Proved on `stage`. Its `usage.md` is descriptions only; every
      signature in it is rendered. `cli.Value(fs, name)` reads a flag back,
      which is what one registration costs at the call site.
- [x] 3. `Verb.Subs`, and render a subcommand's signature the same way.
- [x] 4. Port `secrets`, `deps`, `release`.
- [x] 5. The five cloud verbs, with their exception documented.
- [x] 6. The I7 test: every registered flag appears in the rendered manual.
- [x] 7. Rewrite every `usage.md` to descriptions only.
- [x] 8. Update the `cli` skill; `mise run test`; record results here.

## Results

Every signature in the manual is rendered. `mise run test` exit 0.

`--rotate` was the bug this fixes, and it is now the proof: `dev release DIR
[VERSION] [--keygen] [--name NAME] [--rotate] [--snapshot]` is printed with
nobody having typed any of it. Putting the old hand-typed signature back makes
`TestFlags` fail, naming `--keygen` and `--rotate` as registered and unshown —
so the bug that went unnoticed for a day now cannot survive a test run.

**The exception is one verb, not five.** The plan assumed `url`, `deploy`,
`logs`, `smoke` and `delete` all differ by cloud. Checked verb by verb, four
register identical flags on both; only `smoke` differs, because only a Worker
runs locally under workerd, so a Fly app's takes none of `--path`, `--expect`
or `--timeout`. The signature shows the Worker's set and smoke's description
says so.

**Placeholders come from the standard library.** A backquoted word in a flag's
help names its value, which `flag.UnquoteUsage` reads and `PrintDefaults`
strips. So one string gives `[--env NAME]` in the manual and `-env NAME` under
`--help`, and the old hand-chosen names (`P`, `TEXT`, `NAME`) survived the
port rather than becoming `PATH`, `EXPECT`, `ENV`.

## Issues raised

- This changes `Verb`, which every repo on the stack builds on. auth-proxy's
  `mock-upstream` is the one consumer today and will need porting with it.
