// What somebody looked at and decided not to fix.
//
// A checker is evidence, not an instruction. Some of what it finds is true
// and should be left alone: a manual is one page on purpose, an image really
// is that big, a header really is the host's to set. Until this existed
// there was nowhere to put that decision except a comment in code the
// checker never reads, so every run reported it as open work and every loop
// acting on the report tried again.
//
// That cost something real here. `perf.dom_size.metrics` counts nodes under
// <body> and faulted a page that is a whole manual. An agent closed it by
// splitting the manual into thirty-four pages — the metric improved and the
// site got worse, because a reference is a thing people scan and search and
// no page held the answer any more. Putting it back left the finding open
// again, which is an invitation to do it a second time.
//
// A decision is a declaration, then, with the reason beside it and the date
// it was taken. It does not hide the finding: the report still prints it,
// under its own heading, with why. What it does is stop it counting as work
// nobody has done.
package cli

import (
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/BurntSushi/toml"
)

// Decision is one finding somebody decided to leave, and why.
type Decision struct {
	// Why is the reason, in the words of whoever decided. It is required:
	// a decision with no reason is indistinguishable from an oversight, and
	// the next person to read it cannot tell whether it still holds.
	Why string `toml:"why"`
	// Since is when it was taken, so a reason can be read against what the
	// tree looked like then. Optional, and reported when given.
	Since string `toml:"since"`
}

// Decisions are declined findings by the id they match, read from a file a
// repo keeps beside the thing it is about.
type Decisions map[string]Decision

// ReadDecisions loads a decisions file, or nothing when there is none —
// having declined nothing is the normal state and not an error.
func ReadDecisions(path string) (Decisions, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var got Decisions
	if err := toml.Unmarshal(data, &got); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for id, d := range got {
		if d.Why == "" {
			return nil, fmt.Errorf("%s: %s is declined with no reason; say why, or delete the entry", path, id)
		}
	}
	return got, nil
}

// Claims reports whether this finding is one of the decisions, matching by
// prefix so a family of ids from one checker can be declined together.
func (d Decisions) Claims(id string) (Decision, bool) {
	if got, ok := d[id]; ok {
		return got, true
	}
	for prefix, got := range d {
		if len(prefix) > 0 && prefix[len(prefix)-1] == '.' && len(id) >= len(prefix) && id[:len(prefix)] == prefix {
			return got, true
		}
	}
	return Decision{}, false
}

// Decline marks every finding a decision covers.
//
// Marked rather than removed. A report that drops what it was told to ignore
// is a report that cannot be audited: the next reader cannot see what was
// decided, only that nothing was found, and a decision nobody can review is
// one nobody will revisit when it stops being true.
func (r *Report) Decline(decisions Decisions) {
	if len(decisions) == 0 {
		return
	}
	for i := range r.Findings {
		if got, ok := decisions.Claims(r.Findings[i].ID); ok {
			r.Findings[i].Declined = got.Why
			r.Findings[i].DeclinedOn = got.Since
		}
	}
}

// Declined is what was found and left, with the reason.
func (c Call) Declined(r *Report) {
	left := Filter(r.Findings, func(f Finding) bool { return f.Declined != "" })
	if len(left) == 0 {
		return
	}
	fmt.Fprintf(c.Stdout, "%s found and left on purpose:\n", Plural(len(left), "finding"))
	for _, f := range left {
		when := ""
		if f.DeclinedOn != "" {
			when = " (" + f.DeclinedOn + ")"
		}
		fmt.Fprintf(c.Stdout, "  %-12s %s%s\n      %s\n", f.Tool, Or(f.ID, f.Message), when, f.Declined)
	}
	fmt.Fprintln(c.Stdout)
}

// Stale is every decision that no longer matches anything the run found.
//
// A reason outlives the thing it was about. A decision kept after the
// finding stops appearing is a note about a problem nobody has any more, and
// the next person to read it believes a constraint that is gone.
func Stale(decisions Decisions, r *Report) []string {
	var out []string
	for id := range decisions {
		one := Decisions{id: {}}
		if !slices.ContainsFunc(r.Findings, func(f Finding) bool { _, ok := one.Claims(f.ID); return ok }) {
			out = append(out, id)
		}
	}
	return Sorted(out)
}

// Deciding is the line a reader follows to record one of these.
func Deciding(path string) string {
	return fmt.Sprintf("to leave one on purpose, add it to %s with the reason:\n\n  [\"<the id>\"]\n  why = \"...\"\n  since = %q\n",
		path, time.Now().Format("2006-01-02"))
}
