# The deep audit

`mise run audit:deep` is every check too slow or too noisy for the gate, run
to completion. It fails nothing. This file is the brief for whoever reads it
— a person, or an agent told to go and run it while somebody else is coding.

## Why it is separate

`mise run check` is the inner loop: twenty linters at zero, 223 tests behind
a dead proxy, under five seconds warm. Everything in it is a gate, and a gate
has to be quiet or it gets suppressed.

This is the other trade. It takes minutes, it finds hundreds of things, and
most of them are not bugs. That is not a flaw in it — a net you drag is not a
rule you obey. What it is for is the class of problem the gate cannot catch
without crying wolf: a race that only shows under load, a dependency that
became vulnerable last week, a pattern that is fine in fifty places and wrong
in one.

## What it runs, and what each is actually for

- **govulncheck** — the only check here about the world outside this repo.
  Everything else asks whether the code is right; this asks whether something
  it depends on became dangerous while nobody was looking. Read every hit.
- **the race detector, uncached** — this tree runs goroutines in `Parallel`,
  `Gather` and a memoised `sync.Map`, and a plain `go test` cannot see a data
  race in any of them. A cached pass proves nothing, hence `-count=1`. Read
  every hit: a reported race is real.
- **golangci-lint with every linter** — expect hundreds. `errcheck` alone
  finds 225 and zero were true positives the last time somebody read them,
  because 126 of them are `fmt.Fprintf`, which is this tree's house style.
  Skim for the unfamiliar, ignore the known-noisy.
- **the three standalone reports** — `dead`, `dup`, `repeated`. These see
  across package boundaries where golangci-lint's own copies cannot:
  standalone `goconst` finds 378 cross-package repeats where the gate's copy
  finds none, and standalone `dupl` finds a clone between
  `internal/cloudflare/deploy.go` and `internal/fly/fly.go` that the gate is
  structurally blind to.

## How to read it

The gate is at zero, so **anything here is either noise or something the gate
was not asked to catch**. Sort accordingly:

1. **govulncheck and the race detector first.** Few findings, high value, and
   a hit in either is real until proven otherwise.
2. **Then anything cross-package** from `smells` — a fact spelled in three
   files is the rule this repo cares most about, and it is the one the
   per-package gate cannot see.
3. **Then the wide linter sweep**, looking for classes rather than instances.
   One `errcheck` hit means nothing; forty in one package means that package
   has a habit.

Known noise, already measured and deliberately not gated — do not re-report
these without new evidence: `errcheck` 225 (126 are `fmt.Fprintf`), `gosec`
157 (44 are 0644 file modes, 34 are a CLI reading paths it was given),
`paralleltest` 223 (every test; 13 use `t.Setenv`, which panics under
`t.Parallel`), `noinlineerr` 348 (bans idiomatic Go), `nilerr` 6 (five are
`WalkDir` callbacks skipping unreadable entries).

## What to do with what you find

Do not fix everything. Fix what is real, and for the rest say plainly that it
was read and rejected — a finding somebody has considered is not the same as
one nobody has looked at, and this file is where that distinction lives until
it is worth a `declined.toml` entry.

If something here should have been caught by the gate, that is the more
interesting finding: say which linter would have caught it, whether it sits
at zero on this tree, and whether it should be promoted into `.golangci.yml`.
That is how the gate grows — measured, from something real, rather than by
enabling linters and hoping.
