// Is the deployed thing actually serving, and what is it serving with.
//
// `smoke` runs the command locally and `wait` polls until an address answers.
// Neither says what a deploy is serving right now, and nothing did: checking
// that two clouds serve one site the same way meant reaching for curl by
// hand, comparing two terminal scrollbacks by eye, and believing the result.
//
// A tool that deploys to two clouds has to be able to show what each one
// answers, or the claim that they agree is a claim nobody can check. That is
// the whole of this file: ask the deployed address, and say what came back,
// including the headers — because a header is the half of a response that
// decides how a browser and a crawler treat everything else, and it is the
// half a person never sees without asking for it.
package app

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/joeblew999/dev/cli"
)

// HealthFlags are what `health` takes.
func HealthFlags(fs *flag.FlagSet) {
	fs.String("path", "/", "the `PATH` to ask for")
	fs.String("expect", "", "fail unless the body contains this `TEXT`")
	// The same flags url takes, because it is the same question about the
	// same address: health is url plus asking it.
	URLFlags(fs)
	cli.JSONFlags(fs)
}

// Health is `<cmd> health DIR`: what the deployed app answers, and with what.
type Health struct {
	URL     string            `json:"url"`
	Status  int               `json:"status"`
	TookMs  int64             `json:"tookMs"`
	Headers map[string]string `json:"headers"`
	Body    int               `json:"bodyBytes"`
}

// watched are the headers worth printing without being asked.
//
// Not all of them: a response carries a dozen a reader never decides anything
// from, and a list that prints everything is one nobody reads. These are the
// ones a checker faults a site for, so they are the ones a person is looking
// for when they ask.
var watched = []string{
	"Content-Type",
	"Cache-Control",
	"Content-Security-Policy",
	"Strict-Transport-Security",
	"X-Content-Type-Options",
	"X-Frame-Options",
	"Referrer-Policy",
}

// HealthVerb asks the address this directory deploys to.
func HealthVerb(c cli.Call) error {
	return to(c, "health")
}

// health is the verb's body, run against whichever cloud the directory named.
func health(c cli.Call, t cloud) error {
	url, err := address(c, t)
	if err != nil {
		return err
	}
	url += c.Value("path")
	at := time.Now()
	resp, err := waitClient.Get(url)
	if err != nil {
		// The same diagnosis wait gives, because it is the same failure seen
		// once rather than in a loop.
		return fmt.Errorf("%s: %s", url, diagnose(err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	got := Health{URL: url, Status: resp.StatusCode, TookMs: time.Since(at).Milliseconds(),
		Headers: map[string]string{}, Body: len(body)}
	for _, name := range watched {
		if v := resp.Header.Get(name); v != "" {
			got.Headers[name] = v
		}
	}
	if c.WantsJSON() {
		if err := c.EmitJSON(got); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(c.Stdout, "%s\n  %s in %s, %d bytes\n", got.URL,
			status(got.Status), cli.Took(time.Duration(got.TookMs)*time.Millisecond), got.Body)
		wide := cli.Widest(watched, func(s string) string { return s })
		for _, name := range watched {
			if v, ok := got.Headers[name]; ok {
				fmt.Fprintf(c.Stdout, "  %-*s %s\n", wide, name, v)
			}
		}
		for _, name := range watched {
			if _, ok := got.Headers[name]; !ok {
				fmt.Fprintf(c.Stdout, "  %-*s —\n", wide, name)
			}
		}
	}
	if want := c.Value("expect"); want != "" && !strings.Contains(string(body), want) {
		return fmt.Errorf("%s answered %d and its body does not contain %q", url, got.Status, want)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, status(resp.StatusCode))
	}
	return nil
}
