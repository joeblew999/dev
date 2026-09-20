// Whether the thing in front of an app is set up so the app can be reached.
//
// A project on this stack often deploys to both clouds, because one of them
// cannot do everything, and then Cloudflare stands in front of an app running
// on Fly.
//
// There are several ways to arrange that and no default among them. A tunnel
// is one of them, and naming it first would be a recommendation nobody made.
// What follows is the set, grouped by the thing that actually decides which
// are open to a project: whether it has a domain on this Cloudflare account.
//
// Needing a zone on the account:
//
//   - A proxied DNS record. Cloudflare answers for the hostname and speaks to
//     the origin itself. The plain case, and the cheapest.
//   - A named tunnel. cloudflared dials out from beside the app, so the origin
//     needs no public address at all; the record is a CNAME to
//     <id>.cfargotunnel.com. If the tunnel stops, the record stays and
//     Cloudflare answers 1016 rather than falling back to anything.
//   - A load balancer with origin pools, when there is more than one origin
//     and the point is health checks and steering rather than fronting.
//   - Cloudflare for SaaS, when the hostnames belong to somebody else.
//
// Needing no zone at all:
//
//   - A quick tunnel. cloudflared with no account configuration answers on a
//     random name under trycloudflare.com. Ephemeral and unguessable, which
//     makes it right for a preview and wrong for anything that has to keep
//     its address.
//   - A Worker that fetches the origin. Lives on workers.dev, costs a hop and
//     an invocation per request, and is the only one of these where the thing
//     in front is code somebody wrote.
//
// Two of them share a failure worth naming because nothing about it is
// visible from either side: if Cloudflare speaks plain HTTP to an origin that
// redirects to HTTPS, the request bounces between them until something gives
// up. On a proxied record that is the "flexible" SSL mode; on a tunnel it is
// an http:// service URL. Fly apps redirect by default and the fly.toml this
// repo scaffolds sets force_https, so the pairing is the common case rather
// than an edge.
//
// And one thing to carry into the app itself: behind any of these, the origin
// sees Cloudflare's address rather than the visitor's. The real one is in
// CF-Connecting-IP.
package app

import (
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/cloudflare"
)

// tunnelTarget is what a tunnel's DNS record points at.
const tunnelTarget = ".cfargotunnel.com"

// Fronting reports on what stands in front of the app in dir.
//
// It reads and changes nothing, and that is a rule rather than a stage this
// has not got past. A Cloudflare token that can read a zone's settings can
// usually write them, and this one reaches every zone on the account — so a
// verb that quietly corrected an SSL mode would be changing how every site on
// that domain is served, from a command somebody ran about one app.
//
// Anything here that writes should name what it is writing and be asked for
// by a person who named it too. What it is for is the arrangement that looks
// fine from every angle and does not work: a record that is proxied, a zone
// set to flexible, and an origin that insists on HTTPS.
func Fronting(c cli.Call, host string) error {
	return c.Reported("fronting", host, func(rep *cli.Report) error {
		return fronting(rep, host)
	})
}

// fronting is the questions themselves, against a report somebody else opened
// and will finish. Returning nil is a finished report, which is what a host
// no zone answers for is: an answer, not a failure.
func fronting(rep *cli.Report, host string) error {
	// The zone decides whether there is anything else to ask, so it is asked
	// first and alone. Everything after it is a question about that zone.
	zone, err := cloudflare.ZoneFor(host)
	rep.Record(cli.Measure("zone", "which Cloudflare zone answers for this host",
		func() ([]cli.Finding, string, error) {
			if err != nil {
				return []cli.Finding{{Severity: cli.SevInfo, ID: "no-zone", Message: err.Error(),
					Fix: "nothing on this account fronts it; a Worker that fetches the origin needs no zone, or add the domain to this account"}}, "", err
			}
			return nil, zone.Name, nil
		}))
	if err != nil {
		return nil
	}

	// Both of these are about the same zone and neither depends on the
	// other, so they are asked together.
	var records []cloudflare.Record
	var mode string
	cli.Parts(rep, 2, []cli.Part{
		{Name: "dns", Provides: "what points at this host, and whether Cloudflare stands in front",
			Look: func() ([]cli.Finding, string, error) {
				all, err := cloudflare.Records(zone.ID)
				if err != nil {
					return nil, "", err
				}
				records = cli.Filter(all, func(r cloudflare.Record) bool { return r.Name == host })
				if len(records) == 0 {
					return []cli.Finding{{Severity: cli.SevError, ID: "no-record",
						Message: "no DNS record in " + zone.Name + " for " + host,
						Fix:     "add a CNAME to the origin and proxy it, or a CNAME to the tunnel"}}, "none", nil
				}
				return front(records), cli.Plural(len(records), "record"), nil
			}},
		{Name: "ssl", Provides: "how Cloudflare speaks to the origin",
			Look: func() ([]cli.Finding, string, error) {
				got, err := cloudflare.SSLMode(zone.ID)
				mode = got
				return nil, got, err
			}},
	})

	// Said after both, because it is the two together that are wrong: a
	// flexible zone matters only when something in it is proxied.
	rep.Record(cli.Measure("arrangement", "whether what is there can work",
		func() ([]cli.Finding, string, error) { return sslFindings(mode, records), mode, nil }))

	rep.Fail = host + " is not fronted in a way that will work"
	return nil
}

// front says what arrangement the records describe.
func front(records []cloudflare.Record) []cli.Finding {
	var out []cli.Finding
	for _, r := range records {
		switch {
		case strings.HasSuffix(r.Content, tunnelTarget):
			if !r.Proxied {
				out = append(out, cli.Finding{Tool: "dns", Severity: cli.SevError, ID: "tunnel-not-proxied",
					Message: r.Name + " points at a tunnel and is not proxied, so nothing reaches it",
					Fix:     "a tunnel is only reachable through Cloudflare; turn the record's proxy on"})
				continue
			}
			out = append(out, cli.Finding{Tool: "dns", Severity: cli.SevInfo, ID: "tunnel",
				Message: r.Name + " is served through a tunnel",
				Fix:     "the origin needs no public address; if the tunnel stops, Cloudflare answers 1016 rather than falling back"})
		case !r.Proxied:
			out = append(out, cli.Finding{Tool: "dns", Severity: cli.SevWarning, ID: "not-proxied",
				Message: r.Name + " resolves straight to the origin, so Cloudflare is only its DNS",
				Fix:     "turn the record's proxy on for anything Cloudflare is meant to do in front of it"})
		default:
			out = append(out, cli.Finding{Tool: "dns", Severity: cli.SevInfo, ID: "proxied",
				Message: r.Name + " is proxied to " + r.Content,
				Fix:     "Cloudflare terminates TLS here and speaks to the origin itself, so the SSL mode decides whether that works"})
		}
	}
	return out
}

// sslFindings is the trap: flexible in front of an origin that redirects.
func sslFindings(mode string, records []cloudflare.Record) []cli.Finding {
	proxied := cli.Filter(records, func(r cloudflare.Record) bool { return r.Proxied })
	if len(proxied) == 0 {
		return nil
	}
	if mode == cloudflare.Flexible {
		return []cli.Finding{{
			Tool: "ssl", Severity: cli.SevError, ID: "flexible-in-front-of-https",
			Message: "the zone speaks plain HTTP to the origin, and anything that redirects to HTTPS will loop until a browser gives up",
			Fix:     "set this zone's SSL to Full (strict); a Fly app redirects by default and the fly.toml dev writes sets force_https",
		}}
	}
	if !cli.ToSet(cloudflare.StrictModes)[mode] {
		return []cli.Finding{{
			Tool: "ssl", Severity: cli.SevWarning, ID: "ssl-not-strict",
			Message: "the zone's SSL mode is " + cli.Or(mode, "unset"),
			Fix:     "Full (strict) is what Fly's own documentation asks for in front of a Fly app",
		}}
	}
	return nil
}
