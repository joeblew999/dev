// Which domains on the account point at something, and which do not.
//
// A domain that answers nothing is easy to end up with and hard to notice: it
// was registered for a project that did not happen, or moved somewhere else,
// or was bought defensively beside one that is in use. Nothing tells you —
// the bill is the same and the dashboard shows a healthy zone either way.
package app

import (
	"fmt"
	"time"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/cloudflare"
)

// Domains reports what each zone on the account actually serves.
//
// Read-only, like everything else about zones here: a token that can read a
// zone's records can usually delete them, and this one reaches every zone on
// the account.
func Domains(c cli.Call) error {
	if err := c.CheckReportFlags(); err != nil {
		return err
	}
	started := time.Now()
	rep := cli.NewReport("domains", "this Cloudflare account")
	zones, err := cloudflare.Zones()
	if err != nil {
		return err
	}
	// Asked at once rather than one after another. Each zone is its own
	// request and they do not depend on each other, so doing them in a loop
	// made an account's worth of domains take as long as the sum of them —
	// four and a half seconds here, against one for the same questions asked
	// together.
	type answer struct {
		zone    cloudflare.Zone
		records []cloudflare.Record
		took    time.Duration
		err     error
	}
	work := cli.Map(zones, func(z cloudflare.Zone) func() answer {
		return func() answer {
			at := time.Now()
			records, err := cloudflare.Records(z.ID)
			return answer{zone: z, records: records, took: time.Since(at), err: err}
		}
	})
	// Merged in the registry's order, so the report is the same however they
	// were scheduled — which is what makes two runs comparable.
	for _, got := range cli.Parallel(len(work), work, func(i int, v any) answer {
		return answer{zone: zones[i], err: fmt.Errorf("panicked: %v", v)}
	}) {
		z := got.zone
		step := cli.Step{Name: z.Name, Took: cli.Took(got.took), TookMs: got.took.Milliseconds(),
			Provides: "what this domain points at"}
		if got.err != nil {
			rep.NotRun(step, got.err.Error())
			continue
		}
		serving := cli.Filter(got.records, cloudflare.Record.Serves)
		apex := cli.Filter(serving, func(r cloudflare.Record) bool { return r.Apex(z.Name) })
		step.Covered = fmt.Sprintf("%s, %s at the apex",
			cli.Plural(len(serving), "name"), cli.Plural(len(apex), "record"))
		found := unmapped(z, serving, apex)
		for _, f := range found {
			rep.Add(f)
		}
		step.Findings = len(found)
		rep.Ran(step)
	}
	rep.Fail = "some domains on this account point at nothing"
	return c.Finish(rep, started, func(r *cli.Report) { writeDomains(c, r) })
}

// unmapped is what is worth saying about one zone.
func unmapped(z cloudflare.Zone, serving, apex []cloudflare.Record) []cli.Finding {
	switch {
	case len(serving) == 0:
		return []cli.Finding{{
			Tool: z.Name, Severity: cli.SevWarning, ID: "serves-nothing",
			Message: z.Name + " has no A, AAAA or CNAME that points at anything",
			Fix:     "it is a domain being paid for that answers nothing: point it somewhere, or let it go",
		}}
	case len(apex) == 0:
		return []cli.Finding{{
			Tool: z.Name, Severity: cli.SevInfo, ID: "no-apex",
			Message: z.Name + " serves " + cli.Plural(len(serving), "name") + " and nothing at the bare domain",
			Fix:     "someone typing the domain itself gets nothing; add a record at the apex if that is not deliberate",
		}}
	}
	return nil
}

// writeDomains is the human answer: every domain, and what it serves.
func writeDomains(c cli.Call, r *cli.Report) {
	for _, s := range r.Steps {
		mark := " "
		if s.Findings > 0 {
			mark = "!"
		}
		fmt.Fprintf(c.Stdout, "%s %-24s %s\n", mark, s.Name, cli.Or(s.Covered, s.Note))
	}
	for _, f := range r.Findings {
		fmt.Fprintf(c.Stdout, "\n%-8s %s\n  %s\n  %s\n", f.Severity, f.ID, f.Message, f.Fix)
	}
}
