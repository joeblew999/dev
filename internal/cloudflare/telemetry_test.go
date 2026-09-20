// Reading what the observability API really answered.
//
// The fixture is a real answer, from a Worker deployed with the config this
// package scaffolds, asked to log a line and then asked about. It is here
// because this mapping was written from a guess and the guess was wrong: the
// message and its level are under `source`, not beside the timestamp, so
// every event came back with the right time and an empty message — which
// reads as a Worker that logged nothing, and is the worst kind of wrong,
// because it looks like an answer.
package cloudflare

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func recordedPage(t *testing.T) eventPage {
	t.Helper()
	data, err := os.ReadFile("testdata/telemetry.json")
	if err != nil {
		t.Fatal(err)
	}
	var result queryResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result.Events
}

func TestTheMessageIsReadFromWhereCloudflarePutsIt(t *testing.T) {
	events := recordedPage(t).events()
	if len(events) == 0 {
		t.Fatal("read no events from a real answer")
	}
	for _, e := range events {
		if e.At.IsZero() {
			t.Errorf("an event has no time: %+v", e)
		}
		if e.Message == "" {
			t.Errorf("an event has no message, which is the bug this holds: %+v", e)
		}
	}
	// The Worker's own console.log, which is the whole point: a request line
	// Cloudflare writes for you proves nothing about reading application logs.
	var found bool
	for _, e := range events {
		if strings.Contains(e.Message, "dev telemetry probe") {
			found = true
		}
	}
	if !found {
		t.Error("the line the Worker logged itself is not in what came back")
	}
}

// Everything else here reads oldest first, because that is the order things
// happened in; the API answers newest first.
func TestEventsComeBackInTheOrderThingsHappened(t *testing.T) {
	events := recordedPage(t).events()
	if len(events) < 2 {
		t.Skip("the fixture is too small to order")
	}
	for i := 1; i < len(events); i++ {
		if events[i].At.Before(events[i-1].At) {
			t.Fatalf("event %d is older than the one before it", i)
		}
	}
}

// Workers Logs stores structured logs, so a message may arrive as something
// other than a string. None of those is a reason to lose the line.
func TestAMessageIsALineWhateverShapeItArrivesIn(t *testing.T) {
	for name, tc := range map[string]struct {
		in   any
		want string
	}{
		"a string":       {"hello", "hello"},
		"several parts":  {[]any{"a", "b"}, "a b"},
		"a number":       {float64(42), "42"},
		"nothing at all": {nil, ""},
	} {
		if got := text(tc.in); got != tc.want {
			t.Errorf("%s: text(%v) = %q; want %q", name, tc.in, got, tc.want)
		}
	}
}
