package session

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/session/pins"
	"github.com/joeblew999/dev/internal/session/vendored"
)

// Remove takes back every skill sync put here, in every directory an agent
// reads, and leaves everything else alone.
//
// There was no way to do this, and no way to discover that there was no way.
// Dropping a source from session.toml and syncing again did remove its skills,
// but the move someone actually makes when they want out is to delete
// session.toml — and that left every vendored skill on disk in both
// directories, owned by nothing, loaded by every agent, with sync refusing and
// advising them to fix the file they had just deleted.
//
// What it will not remove is a skill this repo wrote itself. `dev skill`
// writes the manual for each of a command's own verbs, mise links a pinned
// tool's skills, and neither is sync's. The lock is the list of what sync
// owns, and it is the only thing consulted.
func Remove(out io.Writer) error {
	went, err := vendored.Remove()
	if err != nil {
		return err
	}
	if len(went) == 0 {
		fmt.Fprintf(out, "no skills to take back: %s records none.\n", vendored.LockFile)
		return nil
	}
	fmt.Fprintf(out, "took back %s from %s:\n", cli.Plural(len(went), "skill"), english(vendored.Dirs()))
	fmt.Fprint(out, cli.Indent(joinLines(went)))
	if _, err := os.Stat(pins.File); err == nil {
		fmt.Fprintf(out, "\n%s still pins them, so the next sync brings them back; remove what you\ndo not want there first.\n", pins.File)
	}
	// They are off disk and still in memory: an agent reads its skills when it
	// starts, so a session running now has them until it restarts.
	warnStaleSessions(out, time.Now())
	return nil
}

// joinLines is a list as this package shows one, one per line.
func joinLines(names []string) string {
	var out strings.Builder
	for _, name := range names {
		out.WriteString(name + "\n")
	}
	return out.String()
}
