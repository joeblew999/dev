// What a Fly app has been saying, bounded rather than streamed.
//
// `flyctl logs` tails forever, which is right for a person watching a deploy
// and impossible for anything else to use. --no-tail ends it and --json makes
// it readable, and what comes back is not what a reader expects: a stream of
// pretty-printed JSON objects one after another, not one per line. Splitting
// on newlines finds nothing but fragments, which is why this decodes rather
// than scans.
package fly

import (
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/joeblew999/dev/internal/fnox"
)

// Event is one thing an app said. It mirrors app.Event; this package cannot
// import app, which imports it.
type Event struct {
	At      time.Time
	Level   string
	Message string
	Source  string
}

// Events is what the app said in the window, newest last as Fly sends them.
//
// Fly has no flag for how far back or how many, so the window is applied
// here. That is honest about what it is: flyctl decides what it hands over,
// and this keeps the part that was asked for.
func Events(dir string, since time.Duration, limit int) ([]Event, error) {
	app, err := ready(dir)
	if err != nil {
		return nil, err
	}
	said, err := fnox.Ask(".", FlyctlBin, "logs", "--app", app, "--json", "--no-tail")
	if err != nil {
		return nil, err
	}
	return parseEvents(said, time.Now().Add(-since), limit)
}

// parseEvents reads flyctl's concatenated objects, keeping those in the
// window. Split out so a test can run it over output flyctl really produced.
func parseEvents(said string, after time.Time, limit int) ([]Event, error) {
	dec := json.NewDecoder(strings.NewReader(said))
	var out []Event
	for {
		var e struct {
			Timestamp time.Time `json:"timestamp"`
			Level     string    `json:"level"`
			Message   string    `json:"message"`
			Instance  string    `json:"instance"`
			Region    string    `json:"region"`
		}
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			// flyctl prints its own progress before the objects, and a
			// truncated tail is normal when it is cut short. What decoded is
			// the answer; what did not is not worth failing over.
			break
		}
		if e.Timestamp.Before(after) {
			continue
		}
		out = append(out, Event{At: e.Timestamp, Level: e.Level, Message: e.Message, Source: source(e.Instance, e.Region)})
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// source names where a line came from: the machine, and the region when Fly
// says one, because "which of them said this" is the first question when an
// app runs in more than one place.
func source(instance, region string) string {
	switch {
	case instance == "":
		return region
	case region == "":
		return instance
	}
	return instance + " " + region
}
