// Reading what flyctl really printed.
//
// The fixture is output from `flyctl logs --json --no-tail` against an app
// deployed for the purpose, not something composed — because the shape is the
// thing that catches people out here. flyctl prints pretty-printed JSON
// objects one after another, not one per line, so every line-based reader
// finds nothing but fragments and reports no logs at all.
package fly

import (
	"os"
	"strings"
	"testing"
	"time"
)

func recorded(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/logs.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEventsAreDecodedNotScanned(t *testing.T) {
	// Long before the fixture, so the window keeps everything in it.
	events, err := parseEvents(recorded(t), time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("read no events from output flyctl really produced; a line-based reader gets exactly this")
	}
	for _, e := range events {
		switch {
		case e.At.IsZero():
			t.Errorf("an event has no time: %+v", e)
		case e.Message == "":
			t.Errorf("an event has no message: %+v", e)
		case e.Source == "":
			t.Errorf("an event does not say which machine said it: %+v", e)
		}
	}
	// Fly's own words, so a change in the fixture is a change in what Fly
	// sends rather than a change in taste.
	if !strings.Contains(events[0].Message, "image") {
		t.Errorf("the first event was %q; the fixture starts with the image pull", events[0].Message)
	}
}

// The window and the count are applied here, because flyctl has a flag for
// neither. Without them "what just happened" returns everything Fly still
// holds.
func TestTheWindowAndTheLimitAreApplied(t *testing.T) {
	said := recorded(t)
	all, err := parseEvents(said, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 2 {
		t.Skip("the fixture is too small to bound")
	}
	// A window starting after everything keeps nothing.
	future, err := parseEvents(said, time.Now().Add(time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(future) != 0 {
		t.Errorf("a window in the future kept %d events", len(future))
	}
	// A limit keeps the newest, which is what a person asking for the last
	// few means — not the first few.
	one, err := parseEvents(said, time.Time{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 {
		t.Fatalf("limit 1 kept %d", len(one))
	}
	if one[0].At != all[len(all)-1].At {
		t.Error("the limit kept the oldest event; the last few means the newest")
	}
}

// Progress before the objects, and a tail cut short, are both normal. What
// decoded is the answer.
func TestPartialOutputIsNotAFailure(t *testing.T) {
	said := recorded(t)
	if _, err := parseEvents(said[:len(said)/2], time.Time{}, 0); err != nil {
		t.Errorf("a truncated stream failed rather than keeping what it had: %v", err)
	}
	if got, err := parseEvents("waiting for logs...\n", time.Time{}, 0); err != nil || len(got) != 0 {
		t.Errorf("output with no objects gave %v, %v", got, err)
	}
}
