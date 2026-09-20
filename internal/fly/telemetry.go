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

	// Method, URL and Status are empty for everything that is not a request,
	// which is most of a deploy's output: Fly writes the HTTP block on every
	// line and fills it only when there was one.
	Method string
	URL    string
	Status int
	ID     string

	// Provider is Fly's word for who wrote the line: "app" is the
	// application, "runner" is the machinery that starts it.
	Provider string

	// Raw is Fly's own record of the line, kept whole.
	Raw json.RawMessage
}

// Events is what the app said in the window, newest last as Fly sends them.
//
// Fly has no flag for how far back or how many, so the window is applied
// here. That is honest about what it is: flyctl decides what it hands over,
// and this keeps the part that was asked for.
func Events(dir string, since time.Duration, limit int, raw bool) ([]Event, error) {
	app, err := ready(dir)
	if err != nil {
		return nil, err
	}
	said, err := fnox.Ask(".", FlyctlBin, "logs", "--app", app, "--json", "--no-tail")
	if err != nil {
		return nil, err
	}
	return parseEvents(said, time.Now().Add(-since), limit, raw)
}

// parseEvents reads flyctl's concatenated objects, keeping those in the
// window. Split out so a test can run it over output flyctl really produced.
func parseEvents(said string, after time.Time, limit int, raw bool) ([]Event, error) {
	dec := json.NewDecoder(strings.NewReader(said))
	var out []Event
	for {
		// Every object is read as bytes first, whether or not they are
		// kept. Re-encoding the struct below would hand back only the fields
		// this understands, which is the opposite of what raw is for — and
		// reading it twice from the decoder consumes two objects and returns
		// half of them, which is what happened when this was written that
		// way.
		var line json.RawMessage
		if err := dec.Decode(&line); err != nil {
			if err == io.EOF {
				break
			}
			// flyctl prints its own progress before the objects, and a
			// truncated tail is normal when it is cut short. What decoded is
			// the answer; what did not is not worth failing over.
			break
		}
		var e struct {
			Timestamp time.Time `json:"timestamp"`
			Level     string    `json:"level"`
			Message   string    `json:"message"`
			Instance  string    `json:"instance"`
			Region    string    `json:"region"`
			Meta      struct {
				Event struct{ Provider string }
				HTTP  struct {
					Request struct {
						Method string
						ID     string
					}
					Response struct {
						StatusCode int `json:"status_code"`
					}
				}
				URL struct{ Full string }
			} `json:"meta"`
		}
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if e.Timestamp.Before(after) {
			continue
		}
		out = append(out, Event{
			At: e.Timestamp, Level: e.Level, Message: e.Message,
			Source:   source(e.Instance, e.Region),
			Method:   e.Meta.HTTP.Request.Method,
			URL:      e.Meta.URL.Full,
			Status:   e.Meta.HTTP.Response.StatusCode,
			ID:       e.Meta.HTTP.Request.ID,
			Provider: e.Meta.Event.Provider,
			Raw:      kept(line, raw),
		})
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

// AppLine is Fly's provider for a line the application wrote. Everything else
// — "runner" and whatever Fly adds next — is the platform.
const AppLine = "app"

// kept is the line when it was asked for, and nothing otherwise — so an
// answer nobody wanted raw does not carry a copy of itself.
func kept(line json.RawMessage, raw bool) json.RawMessage {
	if raw {
		return line
	}
	return nil
}
