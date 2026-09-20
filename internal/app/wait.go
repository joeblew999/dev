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
func Wait(out io.Writer, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	streak := 0
	for {
		resp, err := waitClient.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				streak++
				if streak == stableFor {
					fmt.Fprintf(out, "%s is up\n", url)
					return nil
				}
				sleep(2 * time.Second)
				continue
			}
		}
		streak = 0
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not answer 200 steadily within %s; look at its logs (dev logs DIR)", url, timeout)
		}
		fmt.Fprintf(out, "waiting for %s to come online...\n", url)
		sleep(5 * time.Second)
	}
}
