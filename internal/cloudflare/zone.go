// The zone in front of an app: DNS, whether Cloudflare proxies it, and the
// SSL mode that decides whether proxying works at all.
//
// None of this is about deploying a Worker. When Cloudflare fronts an app
// running somewhere else — a Fly app, most often here — what Cloudflare owns
// is a record in a zone and the settings around it. That is a different
// question from "where does this directory deploy", and it is asked of a
// different part of the API.
package cloudflare

import (
	"fmt"
	"strings"
)

var (
	zonesEndpoint   = "https://api.cloudflare.com/client/v4/zones?per_page=50&account.id=%s"
	recordsEndpoint = "https://api.cloudflare.com/client/v4/zones/%s/dns_records?per_page=100"
	sslEndpoint     = "https://api.cloudflare.com/client/v4/zones/%s/settings/ssl"
)

// Zone is one domain Cloudflare answers for.
type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Record is one DNS entry, and whether Cloudflare stands in front of it.
type Record struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	// Proxied is the orange cloud: Cloudflare terminates TLS and speaks to
	// the origin itself. Off, it is only DNS and nothing else here matters.
	Proxied bool `json:"proxied"`
}

// Zones is every domain this account answers for.
func Zones() ([]Zone, error) {
	return ask[[]Zone]("the zones on this account", zonesEndpoint)
}

// ZoneFor is the zone a hostname belongs to: the longest zone name it ends
// with, since a.b.example.com belongs to example.com and not to com.
func ZoneFor(host string) (Zone, error) {
	zones, err := Zones()
	if err != nil {
		return Zone{}, err
	}
	best := Zone{}
	for _, z := range zones {
		if host == z.Name || strings.HasSuffix(host, "."+z.Name) {
			if len(z.Name) > len(best.Name) {
				best = z
			}
		}
	}
	if best.ID == "" {
		return Zone{}, fmt.Errorf("no zone on this Cloudflare account answers for %s; it is somebody else's, or the token is for another account", host)
	}
	return best, nil
}

// Records is every DNS entry in a zone.
func Records(zone string) ([]Record, error) {
	return askZone[[]Record]("the DNS records", recordsEndpoint, zone)
}

// SSLMode is how Cloudflare talks to the origin behind a proxied record:
// "off", "flexible", "full", or "strict" — which the dashboard calls
// Full (strict).
func SSLMode(zone string) (string, error) {
	mode, err := askZone[struct {
		Value string `json:"value"`
	}]("the zone's SSL mode", sslEndpoint, zone)
	return mode.Value, err
}

// Flexible is the mode that cannot work in front of an origin that redirects
// to HTTPS: Cloudflare accepts TLS and speaks plain HTTP to the origin, the
// origin redirects to HTTPS, and the request goes round until something gives
// up. Fly apps redirect by default, and the config this repo scaffolds sets
// force_https, so this pairing is not rare — it is the common case.
const Flexible = "flexible"

// StrictModes are the ones that work in front of an origin serving HTTPS.
// "full" reaches the origin over TLS without checking its certificate;
// "strict" checks it, which is what Fly's own documentation asks for.
var StrictModes = []string{"full", "strict"}

// askZone is a zone-scoped GET. The account-scoped ask puts the account id in
// the path; these need the zone instead.
func askZone[T any](what, endpoint, zone string) (T, error) {
	var zero T
	token, err := credential(TokenEnv)
	if err != nil {
		return zero, err
	}
	return request[T](what, fmt.Sprintf(endpoint, zone), token)
}

// Serves reports whether a record actually puts something on the internet.
//
// Only A, AAAA and CNAME point a name at a thing. MX is mail, TXT is proof
// and policy, and neither makes a domain reachable — a zone holding nothing
// but those is a domain somebody is paying for that answers nothing.
//
// Names beginning with an underscore are excluded even when they are CNAMEs.
// They are protocol records — _domainconnect, _acme-challenge, _dmarc — put
// there by a setup flow or a certificate check, and counting one as a site is
// how a domain serving nothing looks like a domain serving something.
func (r Record) Serves() bool {
	switch r.Type {
	case "A", "AAAA", "CNAME":
		return !strings.HasPrefix(r.Name, "_") && !strings.Contains(r.Name, "._")
	}
	return false
}

// Apex reports whether this record is the bare domain rather than a name
// under it. A zone whose subdomains work and whose apex does not is the
// common half-finished state: someone set up www and never the domain
// itself.
func (r Record) Apex(zone string) bool { return r.Name == zone }
