// Reading what flyctl really printed.
//
// The fixture is output from `flyctl logs --json --no-tail` against an app
// deployed for the purpose, not something composed — because the shape is the
// thing that catches people out here. flyctl prints pretty-printed JSON
// objects one after another, not one per line, so every line-based reader
// finds nothing but fragments and reports no logs at all.
//
// The shape is real and the app's name in it is not: it named a repo on this
// stack, and nothing here is allowed to know about one.
package fly

import (
	"net/http"
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
	events, err := parseEvents(recorded(t), time.Time{}, 0, false)
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
	all, err := parseEvents(said, time.Time{}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 2 {
		t.Skip("the fixture is too small to bound")
	}
	// A window starting after everything keeps nothing.
	future, err := parseEvents(said, time.Now().Add(time.Hour), 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(future) != 0 {
		t.Errorf("a window in the future kept %d events", len(future))
	}
	// A limit keeps the newest, which is what a person asking for the last
	// few means — not the first few.
	one, err := parseEvents(said, time.Time{}, 1, false)
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
	if _, err := parseEvents(said[:len(said)/2], time.Time{}, 0, false); err != nil {
		t.Errorf("a truncated stream failed rather than keeping what it had: %v", err)
	}
	if got, err := parseEvents("waiting for logs...\n", time.Time{}, 0, false); err != nil || len(got) != 0 {
		t.Errorf("output with no objects gave %v, %v", got, err)
	}
}

// Fly writes the HTTP block on every line, filled only when the line is about
// a request. Reading it unconditionally would give every deploy message a
// request to nowhere with status 0.
func TestTheHTTPBlockIsOnlyARequestWhenFlyFilledIt(t *testing.T) {
	events, err := parseEvents(recorded(t), time.Time{}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture is a deploy: runner lines, none of them a request.
	for _, e := range events {
		if e.Method != "" || e.URL != "" || e.Status != 0 {
			t.Errorf("a deploy line carries a request: %+v", e)
		}
	}
	// And a line Fly did fill comes through.
	withRequest := `{"timestamp":"2026-09-20T04:23:29.7Z","level":"info","message":"GET /","instance":"abc","region":"lhr",
	  "meta":{"HTTP":{"Request":{"Method":"GET"},"Response":{"status_code":200}},"URL":{"Full":"https://x/"}}}`
	got, err := parseEvents(withRequest, time.Time{}, 0, false)
	if err != nil || len(got) != 1 {
		t.Fatalf("parsed %v (%v)", got, err)
	}
	if got[0].Method != http.MethodGet || got[0].URL != "https://x/" || got[0].Status != 200 {
		t.Errorf("the request was not read: %+v", got[0])
	}
}

// Who wrote a line is the first thing anybody scanning logs wants, because
// most of what comes back is never the application: Fly's image pulls and
// firecracker lines outnumber it during a deploy. Without the distinction,
// "my app logged nothing" and "my app's lines are buried" look the same.
func TestTheProviderSaysWhoWroteTheLine(t *testing.T) {
	events, err := parseEvents(recorded(t), time.Time{}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	var app, platform int
	for _, e := range events {
		switch e.Provider {
		case AppLine:
			app++
		case "":
			t.Errorf("an event does not say who wrote it: %+v", e)
		default:
			platform++
		}
	}
	// The fixture is a real deploy, so it has both: the machinery starting
	// the machine, and the application once it is up.
	if app == 0 || platform == 0 {
		t.Errorf("the fixture has %d app lines and %d platform lines; a deploy has both", app, platform)
	}
}

// Raw keeps the line exactly as the cloud sent it, because the shared shape
// is an envelope and most of what a cloud records has nowhere to go in it —
// Fly's region and provider, Cloudflare's headers and timings. Re-encoding
// the struct would hand back only what was understood, which is the opposite
// of the point.
func TestRawKeepsWhatTheSharedShapeCannotHold(t *testing.T) {
	said := recorded(t)
	without, err := parseEvents(said, time.Time{}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range without {
		if len(e.Raw) != 0 {
			t.Errorf("raw was kept without being asked for: %s", e.Raw)
		}
	}
	with, err := parseEvents(said, time.Time{}, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(with) != len(without) {
		t.Fatalf("asking for raw changed the events: %d vs %d", len(with), len(without))
	}
	for i, e := range with {
		if len(e.Raw) == 0 {
			t.Fatalf("event %d has no raw record", i)
		}
		// The fields still read the same, so raw is an addition rather than
		// a different path through the same output.
		if e.Message != without[i].Message || e.At != without[i].At {
			t.Errorf("event %d differs with raw on: %+v vs %+v", i, e, without[i])
		}
		// And the record holds what the envelope does not: Fly's region,
		// which has no field of its own.
		if !strings.Contains(string(e.Raw), "region") {
			t.Errorf("event %d's raw record lost what only Fly has: %s", i, e.Raw)
		}
	}
}
