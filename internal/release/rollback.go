// Putting back what a failed release made.
//
// A release changes things outside this process before it is public — it
// tags, and it pushes the tag — and a run that dies in between leaves them.
// The next attempt then refuses the tag as already there, which reads as
// "already released" when nothing was. So each step says how to undo itself,
// and a failure runs them in reverse.
package release

import (
	"fmt"
	"io"
	"slices"
)

// rollback is what to put back, newest first.
type rollback []step

type step struct {
	what string
	undo func() error
}

// add records a step that changed something outside this process.
func (r *rollback) add(what string, undo func() error) {
	*r = append(*r, step{what, undo})
}

// done says the work is public and there is nothing left to put back.
func (r *rollback) done() { *r = nil }

// run undoes what was recorded, newest first, saying what it is doing. An
// undo that itself fails is reported and the rest still run: leaving two
// things behind because the first could not be removed helps nobody.
func (r rollback) run(out io.Writer) {
	for _, v := range slices.Backward(r) {
		fmt.Fprintf(out, "putting back: %s\n", v.what)
		if err := v.undo(); err != nil {
			fmt.Fprintf(out, "  could not: %v — do it by hand\n", err)
		}
	}
}
