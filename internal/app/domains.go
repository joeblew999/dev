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
	// Asked at once rather than one after another: each zone is its own
	// request and none depends on another, so a loop made an account's worth
	// of domains take as long as the sum of them.
	cli.Gather(rep, len(zones), zones,
		func(z cloudflare.Zone) string { return z.Name },
		func(z cloudflare.Zone) cli.Measured {
			return cli.Measure(z.Name, "what this domain points at", func() ([]cli.Finding, string, error) {
				records, err := cloudflare.Records(z.ID)
				if err != nil {
					return nil, "", err
				}
				serving := cli.Filter(records, cloudflare.Record.Serves)
				apex := cli.Filter(serving, func(r cloudflare.Record) bool { return r.Apex(z.Name) })
				return unmapped(z, serving, apex), fmt.Sprintf("%s, %s at the apex",
					cli.Plural(len(serving), "name"), cli.Plural(len(apex), "record")), nil
			})
		})
	rep.Fail = "some domains on this account point at nothing"
	return c.Finish(rep, started, c.Write)
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
