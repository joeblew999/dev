// Reading what the observability API really answered.
//
// The fixture is a real answer, from a Worker deployed with the config this
// package scaffolds, asked to log a line and then asked about. It is here
// because this mapping was written from a guess and the guess was wrong: the
// message and its level are under `source`, not beside the timestamp, so
// every event came back with the right time and an empty message — which
// reads as a Worker that logged nothing, and is the worst kind of wrong,
// because it looks like an answer.
//
// The shape is real and the identities in it are not: the account, the
// Worker's name and the visitor's address were replaced. A fixture is kept
// for what Cloudflare's answer looks like, and a test that knows whose
// account it came from is a test that only holds for that account.
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
	events := recordedPage(t).events(false)
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
	events := recordedPage(t).events(false)
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

// Cloudflare writes its own record of every invocation alongside whatever the
// Worker logged, so half of what comes back is never the application. Telling
// them apart is the difference between "my Worker logged nothing" and "my
// Worker's lines are in there among Cloudflare's".
func TestTheTypeSaysWhoWroteTheLine(t *testing.T) {
	events := recordedPage(t).events(false)
	var worker, platform int
	for _, e := range events {
		switch e.Type {
		case WorkerLine:
			worker++
		case "":
			t.Errorf("an event does not say who wrote it: %+v", e)
		default:
			platform++
		}
	}
	if worker == 0 || platform == 0 {
		t.Errorf("the fixture has %d Worker lines and %d Cloudflare lines; a real answer has both", worker, platform)
	}
	// The Worker's own line is the one with the message it logged; the
	// platform's is Cloudflare's request record.
	for _, e := range events {
		if e.Type == WorkerLine && strings.HasPrefix(e.Message, "GET ") {
			t.Errorf("a request record was read as a line the Worker wrote: %+v", e)
		}
	}
}

// The request is what both clouds record, so it is read from a real answer.
// The headers beside it are Cloudflare's alone and deliberately not lifted
// into the shared shape — Fly's log schema has no such field.
func TestTheRequestIsReadAndTheHeadersAreLeftWhereTheyAre(t *testing.T) {
	events := recordedPage(t).events(false)
	var withRequest int
	for _, e := range events {
		if e.Method == "" && e.URL == "" && e.Status == 0 {
			continue
		}
		withRequest++
		if e.Method != "GET" {
			t.Errorf("method = %q; the fixture is a GET", e.Method)
		}
		if !strings.Contains(e.URL, "workers.dev") {
			t.Errorf("url = %q", e.URL)
		}
		// A line the Worker logged carries the request it was serving and no
		// status, because it was written while the request was still being
		// served and there was no response yet. Only Cloudflare's record of
		// the invocation, written after, has the outcome. Both are useful and
		// they are not the same event.
		switch e.Type {
		case WorkerLine:
			if e.Status != 0 {
				t.Errorf("a line the Worker logged carries a status: %+v", e)
			}
		default:
			if e.Status != 200 {
				t.Errorf("the invocation record has status %d; the fixture answered 200", e.Status)
			}
		}
	}
	if withRequest == 0 {
		t.Fatal("no event carried the request Cloudflare recorded")
	}
	// Nothing here reads headers, and the shared Event has nowhere to put
	// them. The fixture keeps them so that is a decision rather than an
	// oversight anyone has to rediscover.
	raw, err := os.ReadFile("testdata/telemetry.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "headers") {
		t.Error("the fixture no longer shows what is being left out")
	}
}
