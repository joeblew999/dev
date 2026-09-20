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
	"encoding/json"
	"time"
)

// Event is one thing an app said.
//
// The named fields are the ones both clouds really have, and they are the
// envelope rather than the whole. What a cloud records about a request is far
// richer than this and not the same on both: Cloudflare keeps the request,
// its headers, the outcome, CPU time and wall time; Fly keeps the region, the
// machine, and which part of its runtime spoke. Flattening those into four
// fields answers "what did it say" and throws away everything needed to work
// out why.
//
// So Raw carries the cloud's own record, exactly as it sent it, for a reader
// that knows which cloud it is asking about. The envelope is the part that
// unifies; the record is not, and pretending it were would mean choosing
// which half of each cloud's telemetry to lose.
type Event struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level,omitempty"` // when the cloud says one
	Message string    `json:"message"`
	Source  string    `json:"source,omitempty"` // the machine or instance, when known

	// From is who said it: the application, or the platform running it. Both
	// clouds record this and it is the first thing anybody scanning logs
	// wants, because half of what comes back is never the application at all
	// — Fly's image pulls and firecracker lines, Cloudflare's own record of
	// each invocation. Without it, "my app logged nothing" and "my app's
	// lines are buried in machinery" look identical.
	//
	// Fly marks it as the provider of the event, runner or app. Cloudflare
	// marks it as the type: an invocation it wrote itself, or a line the
	// Worker wrote.
	From string `json:"from,omitempty"`

	// Request is the one richer thing both clouds really keep, so it is the
	// one worth unifying. Cloudflare records it under the Worker's event and
	// Fly under the machine's HTTP metadata; both say which method, which
	// URL, and what came back. Nil when the event is not about a request,
	// which most of a deploy's output is not.
	Request *Request `json:"request,omitempty"`

	// Raw is omitted unless asked for. Cloudflare's record of one request is
	// thousands of bytes of TLS and geography, so a hundred of them by
	// default would bury the thing somebody came to read.
	Raw json.RawMessage `json:"raw,omitempty"`
}

// Request is an HTTP request as both clouds record it.
//
// These and no more, because these are what both have. Headers are the
// clearest case of what is left out: Cloudflare records them under the
// Worker's request, and Fly's log schema has no headers at all — it emits its
// whole metadata block with empty values, and across every key it writes
// there is no such field. So headers are real, useful, and Cloudflare's, and
// they are in Raw rather than in a field that would be permanently empty for
// half the stack. Cloudflare
// also keeps CPU time, wall time, an outcome and the script version; Fly keeps
// the region, the machine and which part of its runtime spoke. Adding either
// side's extras here would mean a field that is always empty for the other,
// which reads as missing data rather than as a difference between clouds —
// and both are in Raw for a reader who wants them.
type Request struct {
	Method string `json:"method,omitempty"`
	URL    string `json:"url,omitempty"`
	Status int    `json:"status,omitempty"`

	// ID is what the cloud called this request: Fly's request id, or
	// Cloudflare's. It is here because it is the field that makes the rest
	// worth having — one line about a request is a fact, and an id is what
	// lets somebody find the others about the same one.
	ID string `json:"id,omitempty"`
}

// Any reports whether anything was recorded, so an empty request — which Fly
// writes for every line that is not about one — is left out rather than
// shown as a request to nowhere.
func (r Request) Any() bool {
	return r.Method != "" || r.URL != "" || r.Status != 0 || r.ID != ""
}

// Telemetry is what a bounded ask takes: how far back, and how many at most.
//
// Both have to be bounded. Cloudflare keeps seven days and a query with no
// limit reads every row of it — the probe query here scanned three and a half
// million — and Fly's stream has no end at all.
type Telemetry struct {
	Since time.Duration
	Limit int

	// Raw asks each event to carry the cloud's own record as well. Off by
	// default because it is large; on when somebody is debugging rather than
	// reading.
	Raw bool
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

// request is the shared shape, or nil when the cloud recorded nothing about
// one — which is most lines. Both targets build it the same way, so the rule
// for what counts as "nothing" is in one place.
func request(method, url, id string, status int) *Request {
	r := Request{Method: method, URL: url, Status: status, ID: id}
	if !r.Any() {
		return nil
	}
	return &r
}

// at is a time as the shared shape reports it, which is UTC.
//
// The envelope is the part that unifies, and it was not unified here: Fly
// sends an RFC 3339 time and it stayed UTC, Cloudflare sends milliseconds and
// time.UnixMilli gives them the machine's own zone — so the same `dev logs
// --json` answered in two different zones depending on which cloud it asked.
// A reader comparing two apps has no way to see that from the output.
func at(t time.Time) time.Time { return t.UTC() }

// Who said a line: the application itself, or the platform running it.
const (
	FromApp      = "app"
	FromPlatform = "platform"
)

// from is the shared answer, so the rule for reading each cloud's own word
// for it lives with the shape rather than in two targets.
func from(itsOwn bool) string {
	if itsOwn {
		return FromApp
	}
	return FromPlatform
}
