// What resolves a finding, and running it.
//
// A report says what is wrong. Some of what is wrong is something the same
// command can put right — `seo check` finds a missing canonical and `seo
// write` writes the file that has one; `session check` finds a skill that
// drifted and `session sync` writes it back. Every command on this stack
// that has a check has something that fixes what the check finds, because
// that pair is what AGENTS.md calls reconcile and establish, and it keeps
// recurring.
//
// It was being written out per command, and badly. seo had the whole
// mechanism — ids routed to writers by prefix, a summary of what was
// fixable — and kept it private to one package. session had the same
// relationship and expressed it as a sentence: every finding's Fix ended
// with "fix with: dev session sync", a string nothing could run, repeated
// on every finding. A reader could see the fix and not take it.
//
// So the shape lives here, in the API every command is built on, and the
// work stays with the command. cli decides which fixer claims a finding and
// says so; what a fixer does when it runs is nobody's business but its own.
package cli

import (
	"fmt"
	"strings"
)

// Fixer is something in this same command that resolves findings.
type Fixer struct {
	// Name is what to call it in a report, and it is the verb or the writer
	// a reader would go and run.
	Name string
	// Produces is what it makes, when it makes a file. Empty for a fixer
	// that changes something else — a settings key, a directory's contents.
	Produces string
	// Fixes are the finding ids this claims, matched by prefix so that a
	// family of ids from one checker lands on one fixer: kitsune's
	// seo.canonical.* and scoutly's missing-canonical are the same problem
	// and the same file answers both.
	Fixes []string
	// Run closes the loop. A fixer with no Run is one a reader has to run
	// themselves, which is still worth naming — it is the difference between
	// "this is wrong" and "this is wrong and that is what fixes it".
	Run func(Call, *Report) error
}

// Claims reports whether this fixer answers a finding with that id.
func (f Fixer) Claims(id string) bool {
	for _, prefix := range f.Fixes {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

// Attribute names, on every finding, the fixer that resolves it.
//
// Done in one pass over the whole report rather than as each finding is
// made, because a checker does not know what fixes what — it knows what it
// found. Left to each producer this was done in some and not others, and a
// finding that nothing claimed was indistinguishable from one nothing could
// fix.
func Attribute(r *Report, fixers []Fixer) {
	for i := range r.Findings {
		for _, f := range fixers {
			if f.Claims(r.Findings[i].ID) {
				r.Findings[i].FixedBy = f.Name
				break
			}
		}
	}
}

// Claimed is the fixers this report's findings ask for, in the order they
// were declared, with how many findings each answers.
func Claimed(r *Report, fixers []Fixer) []Fixer {
	wanted := CountBy(Filter(r.Findings, func(f Finding) bool { return f.FixedBy != "" }),
		func(f Finding) string { return f.FixedBy })
	return Filter(fixers, func(f Fixer) bool { return wanted[f.Name] > 0 })
}

// Fixable says what the report holds that this command can put right, and
// how to ask for it.
//
// Printed even when nothing will be run, because knowing a thing is fixable
// is most of the value: the alternative is a list of faults and no route out
// of it, which is the state `seo check` was in while holding every piece of
// the answer.
func (c Call) Fixable(r *Report, fixers []Fixer, how string) []Fixer {
	claimed := Claimed(r, fixers)
	if len(claimed) == 0 {
		return nil
	}
	answered := len(Filter(r.Findings, func(f Finding) bool { return f.FixedBy != "" }))
	fmt.Fprintf(c.Stdout, "%d of %s this command can put right:\n",
		answered, Plural(len(r.Findings), "finding"))
	wanted := CountBy(r.Findings, func(f Finding) string { return f.FixedBy })
	wide := Widest(claimed, func(f Fixer) string { return f.Name })
	for _, f := range claimed {
		fmt.Fprintf(c.Stdout, "  %-*s %-16s fixes %d\n", wide, f.Name, f.Produces, wanted[f.Name])
	}
	if how != "" {
		fmt.Fprintf(c.Stdout, "\n  %s\n\n", how)
	}
	return claimed
}

// Fix runs every fixer the findings asked for.
//
// Only what was asked for: running all of them would overwrite something
// somebody wrote to answer a finding that is no longer there, and the whole
// value of a fix that reads a report is that it touches what the report is
// about.
func (c Call) Fix(r *Report, fixers []Fixer) error {
	claimed := Claimed(r, fixers)
	runnable := Filter(claimed, func(f Fixer) bool { return f.Run != nil })
	switch {
	case len(claimed) == 0:
		fmt.Fprintf(c.Stderr, "nothing found that this command can fix\n")
		return nil
	case len(runnable) == 0:
		// Named rather than silently skipped: a fixer with no Run is a real
		// answer that this command cannot carry out for you.
		fmt.Fprintf(c.Stderr, "%s answers this, and has to be run yourself\n",
			English(Map(claimed, func(f Fixer) string { return f.Name })))
		return nil
	}
	fmt.Fprintf(c.Stderr, "fixing %s\n", English(Map(runnable, func(f Fixer) string { return f.Name })))
	for _, f := range runnable {
		if err := f.Run(c, r); err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
	}
	return nil
}
