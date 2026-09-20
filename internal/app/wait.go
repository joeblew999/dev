// Waiting for a deployed app to answer.
//
// Nothing here is about one cloud. It lived in internal/cloudflare, where it
// was a plain HTTP poller among wrangler calls, and `dev wait` reached into
// that package to get it — as did every `deploy --wait`, including a deploy to
// Fly. A Fly deploy calling a function in the Cloudflare package is the kind
// of thing that makes a reader stop trusting what the directories mean.
package app

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// stableFor is how many polls in a row must answer 200 before an app counts as
// up: a hostname that has just been (re)deployed answers on one edge and an
// error on the next for a short while, so a single success proves nothing.
const stableFor = 3

// waitClient is this package's own, so moving here took nothing with it. The
// timeout is per request; the caller's deadline governs the whole wait.
var waitClient = &http.Client{Timeout: 30 * time.Second}

// sleep is a variable so a test can make the waiting instant.
var sleep = time.Sleep

// Wait polls url until it answers 200 stableFor times in a row.
//
// What it saw last is kept, because "did not answer 200 steadily" was the same
// sentence for a redirect loop, a certificate that is not issued yet, a 502
// from a machine that never started and a name that resolves nowhere. Those
// have four different fixes and the message named none of them.
func Wait(out io.Writer, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	streak, last := 0, "nothing yet"
	for {
		resp, err := waitClient.Get(url)
		switch {
		case err != nil:
			last = diagnose(err)
		case resp.StatusCode == http.StatusOK:
			resp.Body.Close()
			streak++
			if streak == stableFor {
				fmt.Fprintf(out, "%s is up\n", url)
				return nil
			}
			sleep(2 * time.Second)
			continue
		default:
			resp.Body.Close()
			last = status(resp.StatusCode)
		}
		streak = 0
		if time.Now().After(deadline) {
			return fmt.Errorf("%s never answered steadily within %s: %s; look at its logs (dev logs DIR)", url, timeout, last)
		}
		fmt.Fprintf(out, "waiting for %s to come online (%s)...\n", url, last)
		sleep(5 * time.Second)
	}
}

// diagnose turns a request failure into the thing to go and fix.
//
// Each of these is a real way a deployed app fails to answer, and each has a
// different fix. The redirect loop is worth naming exactly: a CDN in front of
// an app that forces HTTPS, terminating TLS and speaking plain HTTP to the
// origin, bounces the request between them forever. On Cloudflare that is the
// "Flexible" SSL mode, and the setting wanted is "Full (strict)" — which is
// not something anybody guesses from "did not answer 200".
func diagnose(err error) string {
	said := err.Error()
	switch {
	case strings.Contains(said, "stopped after") && strings.Contains(said, "redirect"):
		return "redirected in a loop, which is what a CDN in front of an app that forces HTTPS does when it speaks plain HTTP to the origin (on Cloudflare: set SSL to Full (strict), not Flexible)"
	case strings.Contains(said, "x509") || strings.Contains(said, "certificate"):
		return "its certificate is not valid yet, which is normal for a minute after a custom domain is added (fly certs check DOMAIN)"
	case strings.Contains(said, "no such host"):
		return "the name resolves nowhere, so nothing points at it yet (check the DNS record)"
	case strings.Contains(said, "connection refused"):
		return "nothing is listening, so the machine is not up or is on another port"
	case strings.Contains(said, "timeout") || strings.Contains(said, "deadline"):
		return "it did not answer in time"
	}
	return said
}

// status is a reply as a person would read it, and says who is complaining
// when that is knowable.
//
// The 52x range is not the app's: those are Cloudflare's own codes for "I
// could not reach the origin", so the thing to look at is between the CDN and
// the app rather than in the app. Reading 521 as an application error sends
// somebody into logs that say nothing, because the request never arrived.
func status(code int) string {
	said := fmt.Sprintf("answered %d", code)
	if text := http.StatusText(code); text != "" {
		said += " " + text
	}
	switch code {
	case 520:
		return said + ", which is Cloudflare saying the origin gave it something it could not read"
	case 521:
		return said + ", which is Cloudflare saying the origin refused it — the app is stopped, or not listening on the port its config names"
	case 522:
		return said + ", which is Cloudflare saying the origin never answered — check the app is running and reachable from outside"
	case 523:
		return said + ", which is Cloudflare saying it cannot find the origin at all — the DNS record points somewhere that is not there"
	case 525, 526:
		return said + ", which is Cloudflare saying TLS to the origin failed — with Full (strict) the origin needs a certificate Cloudflare trusts"
	}
	return said
}
