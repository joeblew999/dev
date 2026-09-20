// A deployed app answering, which is not about any one cloud.
package app

import (
	"bytes"
	"errors"
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
	if err := Wait(&out, down.URL, 0); err == nil || !strings.Contains(err.Error(), "never answered steadily") {
		t.Fatalf("got %v", err)
	}
}

// "did not answer 200 steadily" was one sentence for a redirect loop, an
// unissued certificate, a 502 from a machine that never started and a name
// that resolves nowhere. Four failures, four different fixes, and the message
// named none of them — which matters most for the arrangement this stack
// actually uses, a Fly app behind Cloudflare, where the commonest failures
// are not the app's at all.
func TestWaitSaysWhatItActuallySaw(t *testing.T) {
	for name, tc := range map[string]struct{ err, want string }{
		"a redirect loop": {
			err:  `Get "https://x/": stopped after 10 redirects`,
			want: "Full (strict)",
		},
		"a certificate not issued yet": {
			err:  `x509: certificate is valid for a, not b`,
			want: "fly certs check",
		},
		"a name pointing nowhere": {
			err:  `dial tcp: lookup x: no such host`,
			want: "resolves nowhere",
		},
		"nothing listening": {
			err:  `dial tcp 1.2.3.4:443: connect: connection refused`,
			want: "nothing is listening",
		},
	} {
		if got := diagnose(errors.New(tc.err)); !strings.Contains(got, tc.want) {
			t.Errorf("%s: diagnose said %q; want it to mention %q", name, got, tc.want)
		}
	}
	// An error nobody has a sentence for is passed through rather than
	// swallowed, because a wrong guess is worse than the raw text.
	if got := diagnose(errors.New("something new")); got != "something new" {
		t.Errorf("an unknown failure became %q", got)
	}
}

// The 52x codes are Cloudflare's own, not the app's: the request never
// reached the app, so reading them as application errors sends somebody into
// logs that say nothing.
func TestCloudflareOriginCodesAreNamedAsCloudflares(t *testing.T) {
	for code, want := range map[int]string{
		521: "refused",
		522: "never answered",
		523: "cannot find the origin",
		525: "TLS to the origin failed",
	} {
		got := status(code)
		if !strings.Contains(got, "Cloudflare") || !strings.Contains(got, want) {
			t.Errorf("status(%d) = %q; want it to name Cloudflare and %q", code, got, want)
		}
	}
	// An ordinary code is an ordinary sentence, with no story attached.
	if got := status(503); !strings.Contains(got, "503") || strings.Contains(got, "Cloudflare") {
		t.Errorf("status(503) = %q; want it plain", got)
	}
}
