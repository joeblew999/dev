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
	fmt.Fprintf(out, "took back %s from %s:\n", cli.Plural(len(went), "skill"), cli.English(vendored.Dirs()))
	fmt.Fprint(out, cli.Indent(strings.Join(went, "\n")))
	if _, err := os.Stat(pins.File); err == nil {
		fmt.Fprintf(out, "\n%s still pins them, so the next sync brings them back; remove what you\ndo not want there first.\n", pins.File)
	}
	// Off disk is not the same as out of a running session: one open since
	// before this still offers what was just taken back. A reload is enough to
	// settle it — tested against a live process, which stopped having a skill
	// after its directory went and /reload-skills was sent — so the warning
	// names that rather than a restart.
	warnStaleSessions(out, time.Now())
	return nil
}
