// What a deployed app has been saying, bounded and structured.
//
// `dev logs` attaches a stream to the terminal and runs until interrupted,
// which is right for a person watching a deploy and useless to everything
// else: a script cannot end it, a report cannot include it, and an agent
// cannot read it at all. Both clouds can answer the bounded question instead
// — the last N events over the last D — and this is that question, asked the
// same way whichever cloud is underneath.
//
// The two are not the same underneath and never will be. Fly streams from its
// machines and `flyctl logs` takes --json and --no-tail; Cloudflare stores
// Workers Logs for seven days and answers a query over them. What they share
// is the shape of the answer, which is what a caller wants.
package app

import (
	"time"
)

// Event is one thing an app said. The fields are the ones both clouds really
// have — anything richer belongs to one of them and would be empty for the
// other, which is worse than absent.
type Event struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level,omitempty"` // when the cloud says one
	Message string    `json:"message"`
	Source  string    `json:"source,omitempty"` // the machine or instance, when known
}

// Telemetry is what a bounded ask takes: how far back, and how many at most.
//
// Both have to be bounded. Cloudflare keeps seven days and a query with no
// limit reads every row of it — the probe query here scanned three and a half
// million — and Fly's stream has no end at all.
type Telemetry struct {
	Since time.Duration
	Limit int
}

// Defaults fills in what a caller did not say. An hour and a hundred lines is
// what someone asking "what just happened" means.
func (t Telemetry) Defaults() Telemetry {
	if t.Since <= 0 {
		t.Since = time.Hour
	}
	if t.Limit <= 0 {
		t.Limit = 100
	}
	return t
}
