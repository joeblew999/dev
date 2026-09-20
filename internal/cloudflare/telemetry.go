// Workers Logs, asked as a bounded query rather than tailed.
//
// `wrangler tail` streams what a Worker says from now on, which cannot answer
// "what happened ten minutes ago" and cannot end. Workers Logs keeps seven
// days of it, and the observability API answers over that — but only for a
// Worker whose config turns it on, which is why the scaffold here does.
package cloudflare

import (
	"fmt"
	"strings"
	"time"
)

// telemetryEndpoint runs one query over the account's stored logs.
var telemetryEndpoint = "https://api.cloudflare.com/client/v4/accounts/%s/workers/observability/telemetry/query"

// logsDataset is where Workers Logs are kept.
const logsDataset = "cloudflare-workers"

// Events is what the Worker said, most recent first.
//
// Scoped to this one Worker by $metadata.service, which is the script name.
// Without that filter the query reads the whole account — every Worker, every
// day kept — which on this account is millions of rows for one question about
// one script.
func Events(name string, since time.Duration, limit int) ([]Event, error) {
	to := time.Now()
	from := to.Add(-since)
	body := map[string]any{
		// The API wants an id for the query. It names this run, and a name
		// saying where it came from is kinder than a uuid to anyone reading
		// the account's query history.
		"queryId":   "dev-logs",
		"timeframe": map[string]int64{"from": from.UnixMilli(), "to": to.UnixMilli()},
		"limit":     limit,
		"view":      "events",
		"parameters": map[string]any{
			"datasets": []string{logsDataset},
			"filters": []map[string]any{{
				"key": "$metadata.service", "operation": "eq", "type": "string", "value": name,
			}},
		},
	}
	result, err := post[queryResult](fmt.Sprintf("what %s has been saying", name), telemetryEndpoint, body)
	if err != nil {
		return nil, err
	}
	return result.Events.events(), nil
}

// queryResult is what the observability API answers with.
//
// The line and its level are under `source`, not beside the timestamp — which
// is a thing to be read off a real answer rather than assumed, and was
// assumed here first: every event came back with the right time and an empty
// message, which reads as a Worker that logged nothing.
type queryResult struct {
	Events eventPage `json:"events"`
}

type eventPage struct {
	Count  int `json:"count"`
	Events []struct {
		Timestamp int64 `json:"timestamp"`
		Source    struct {
			Level   string `json:"level"`
			Message any    `json:"message"`
		} `json:"source"`
	} `json:"events"`
}

// events is the page as this package hands it back, newest last.
func (p eventPage) events() []Event {
	out := make([]Event, 0, len(p.Events))
	for _, e := range p.Events {
		out = append(out, Event{
			At:      time.UnixMilli(e.Timestamp),
			Level:   e.Source.Level,
			Message: text(e.Source.Message),
		})
	}
	// The API answers newest first; everything else here reads oldest first,
	// because that is the order things happened in.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Event is one thing a Worker said. It mirrors app.Event; this package cannot
// import app, which imports it.
type Event struct {
	At      time.Time
	Level   string
	Message string
}

// text is a log message as a line, whatever shape it arrived in: Workers Logs
// stores structured logs, so a message may be an object rather than a string.
func text(v any) string {
	switch m := v.(type) {
	case nil:
		return ""
	case string:
		return m
	case []any:
		parts := make([]string, 0, len(m))
		for _, p := range m {
			parts = append(parts, text(p))
		}
		return join(parts, " ")
	default:
		return fmt.Sprint(m)
	}
}

func join(parts []string, sep string) string {
	var out strings.Builder
	for i, p := range parts {
		if i > 0 {
			out.WriteString(sep)
		}
		out.WriteString(p)
	}
	return out.String()
}
