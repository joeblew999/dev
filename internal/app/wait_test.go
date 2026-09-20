// A deployed app answering, which is not about any one cloud.
package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWait(t *testing.T) {
	oldSleep := sleep
	sleep = func(time.Duration) {}
	t.Cleanup(func() { sleep = oldSleep })
	n := 0
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	srv.Start()
	var out bytes.Buffer
	if err := Wait(&out, srv.URL, time.Minute); err != nil {
		t.Fatal(err)
	}
	if n != 2+stableFor || strings.Count(out.String(), "waiting for") != 2 {
		t.Fatalf("n=%d out=%q", n, out.String())
	}
	down := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(530) }))
	down.Start()
	if err := Wait(&out, down.URL, 0); err == nil || !strings.Contains(err.Error(), "did not answer 200") {
		t.Fatalf("got %v", err)
	}
}
