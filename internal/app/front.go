// Putting Cloudflare in front of an app, through opentofu.
//
// Written by hand first — two API calls and a preview built from reads this
// package already does — and that was the right shape for two resources and
// the wrong one for where this is going. The Cloudflare surface a project
// here will want is vast: Access, WAF, load balancers, certificates, routes.
// Forty lines and a test each, with the field names guessed from a response,
// is a bad trade against a provider Cloudflare generates and tests.
//
// So dev writes the HCL and drives opentofu, and nobody on this stack writes
// Terraform by hand. Both are binaries the registry fetches, so this costs
// go.mod nothing — which is the whole reason it is tofu and not the Go SDK.
//
// What that buys beyond breadth: plan says exactly what would change before
// anything does, state records what dev made so removing it is exact, and the
// provider knows things this would otherwise find out the hard way — that a
// zone setting cannot be destroyed, only set to something else.
package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
	"github.com/joeblew999/dev/internal/cloudflare"
	"github.com/joeblew999/dev/internal/fly"
	"github.com/joeblew999/dev/internal/fnox"
)

// TofuBin is the binary that applies what dev writes.
const TofuBin = "tofu"

// stateDir is where the record of what dev made lives, beside the repo and
// gitignored: it describes an account rather than the code, and it holds
// resource ids that are nobody else's business.
const stateDir = ".dev/fronting"

// pluginCache is one provider copy per machine rather than per repo. The
// Cloudflare provider is 250 MB, and a stack of repos each downloading their
// own is the difference between 250 MB and a gigabyte an hour.
func pluginCache() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "dev", "tofu-plugins")
	}
	return ""
}

// Front puts Cloudflare in front of host, or says what it would do.
//
// The zone is named rather than inferred. A token here reaches every zone on
// the account, and "work out which zone this hostname belongs to and change
// its settings" is a sentence that should make anybody nervous — so the zone
// is something a person typed, and a mismatch is a refusal.
func Front(c cli.Call, host, origin string) error {
	// The origin is the app's own address, and Fly needs the app's name to
	// issue a certificate for the host. `<app>.fly.dev` is the one, and
	// anything else is a host Fly does not know about.
	app := strings.TrimSuffix(origin, ".fly.dev")
	if app == origin {
		return cli.Usagef("the origin is %q; fronting a certificate needs a Fly app, which is <app>.fly.dev", origin)
	}
	// The Fly provider wants the organisation named. flyctl knows which ones
	// these credentials reach, so it is asked rather than the person — and
	// when there is exactly one there is nothing to choose.
	org := c.Value("org")
	if org == "" {
		orgs, err := fly.Orgs()
		if err != nil {
			return err
		}
		switch len(orgs) {
		case 1:
			org = orgs[0]
		case 0:
			return errors.New("flyctl reports no organisation, so the Fly provider cannot be told one; check FLY_API_TOKEN in fnox")
		default:
			return cli.Usagef("these credentials reach %s; name the one this app is in with --org", cli.English(orgs))
		}
	}
	zone, err := named(c, host)
	if err != nil {
		return err
	}
	dir, err := workspace(host)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(config(zone, host, origin, app, org)), 0o644); err != nil {
		return err
	}
	if err := tofu(c, dir, "init", "-input=false", "-no-color"); err != nil {
		return err
	}
	// Plan always, apply only when asked. The plan is the answer to "what is
	// about to happen to a zone I share with everything else I run".
	if err := tofu(c, dir, "plan", "-input=false", "-no-color"); err != nil {
		return err
	}
	if !c.Given("apply") {
		fmt.Fprintf(c.Stdout, "\nNothing changed. Pass --apply to make it so.\n")
		return nil
	}
	if err := tofu(c, dir, "apply", "-auto-approve", "-input=false", "-no-color"); err != nil {
		return err
	}
	fmt.Fprintf(c.Stdout, "\n%s is fronted. It may take a minute: dev wait https://%s/\n", host, host)
	return nil
}

// Unfront removes what dev made, by destroying what its state records — which
// is why this needs no guessing about which record was ours.
//
// The zone's SSL mode stays. Terraform cannot destroy a zone setting, and
// strict is right for the zone whether or not this app is still in front of
// it.
func Unfront(c cli.Call, host string) error {
	if _, err := named(c, host); err != nil {
		return err
	}
	dir, err := workspace(host)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "main.tf")); err != nil {
		return fmt.Errorf("dev did not front %s from this repo; there is nothing here to remove", host)
	}
	if err := tofu(c, dir, "init", "-input=false", "-no-color"); err != nil {
		return err
	}
	args := []string{"destroy", "-input=false", "-no-color"}
	if c.Given("yes") {
		args = append(args, "-auto-approve")
	}
	if err := tofu(c, dir, args...); err != nil {
		return err
	}
	fmt.Fprintf(c.Stdout, "\nremoved. The zone's SSL mode is left as it is: Terraform cannot unset one, and strict is right for the zone either way.\n")
	return nil
}

// named resolves the zone and refuses to act on one nobody typed.
func named(c cli.Call, host string) (cloudflare.Zone, error) {
	zone, err := cloudflare.ZoneFor(host)
	if err != nil {
		return zone, err
	}
	switch got := c.Value("zone"); got {
	case zone.Name:
		return zone, nil
	case "":
		return zone, cli.Usagef("name the zone with --zone %s; this changes how Cloudflare serves every site on it, so it is not inferred from a hostname", zone.Name)
	default:
		return zone, cli.Usagef("--zone is %q and %s belongs to %s; name the zone this is really about", got, host, zone.Name)
	}
}

// workspace is where this host's configuration and state live.
func workspace(host string) (string, error) {
	dir := filepath.Join(stateDir, host)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// State holds resource ids for somebody's account. It is not the repo's
	// code and does not belong in its history.
	return dir, cli.Ignore(".", stateDir)
}

// tofu runs one command, with the token in its environment the way every
// other tool here gets one.
func tofu(c cli.Call, dir string, args ...string) error {
	env := []string{}
	if cache := pluginCache(); cache != "" {
		if err := os.MkdirAll(cache, 0o755); err == nil {
			env = append(env, "TF_PLUGIN_CACHE_DIR="+cache)
		}
	}
	return tool.Cmd{
		Bin: fnox.Bin, Dir: dir, Env: env,
		Args: append([]string{"exec", "--", TofuBin}, args...),
	}.Stream(c.Stdout)
}

// config is the whole arrangement, written out so what tofu does is readable
// rather than implied.
//
// Both clouds in one plan, which is the thing worth having. A custom domain
// on a Fly app behind Cloudflare is not two jobs that happen to be adjacent:
// Fly issues the certificate and says what DNS it needs to prove ownership,
// and Cloudflare is what answers for that DNS. The Fly provider computes
// those values and the Cloudflare provider consumes them, so the dependency
// is expressed rather than performed in the right order by hand.
//
// Which also settles the ordering problem that makes this fiddly by hand:
// with Cloudflare proxying, Fly cannot use its usual TLS-ALPN challenge,
// because Cloudflare terminates TLS. It needs the ownership record instead —
// and that record has to exist before validation will pass. tofu works that
// out from the references.
func config(zone cloudflare.Zone, host, origin, app, org string) string {
	name := strings.TrimSuffix(strings.TrimSuffix(host, zone.Name), ".")
	if name == "" {
		name = "@"
	}
	return `# Written by ` + "`dev front`" + `. Edit it if you need something this does not
# do; dev writes it only when it is not there, and leaves your changes alone.
terraform {
  required_providers {
    cloudflare = { source = "cloudflare/cloudflare", version = "~> 5.0" }
    fly        = { source = "ampbase-io/fly", version = "~> 0.2" }
  }
}

# Both take their token from the environment, which fnox supplies.
provider "cloudflare" {}
provider "fly" {
  org_slug = "` + org + `"
}

# Fly issues the certificate and computes what DNS it needs to prove the
# domain is yours.
resource "fly_cert" "app" {
  app      = "` + app + `"
  hostname = "` + host + `"
}

# Cloudflare answers for that DNS. Terraform manages what is declared and
# nothing else, so the rest of this zone is untouched.
resource "cloudflare_dns_record" "app" {
  zone_id = "` + zone.ID + `"
  name    = "` + name + `"
  type    = "CNAME"
  content = "` + origin + `"
  proxied = true
  ttl     = 1
}

# Proving the domain is yours. With Cloudflare proxying, Fly cannot use its
# usual TLS challenge — Cloudflare terminates TLS before Fly ever sees it —
# so ownership is proved by this record instead. It is not proxied: it is a
# fact to be read, not a site to be served.
resource "cloudflare_dns_record" "ownership" {
  zone_id = "` + zone.ID + `"
  name    = fly_cert.app.ownership_name
  type    = "TXT"
  content = "\"${fly_cert.app.ownership_app_value}\""
  ttl     = 60
}

# Without this, Cloudflare speaks plain HTTP to an origin that redirects to
# HTTPS, and the request bounces between them until a browser gives up. A Fly
# app redirects by default and the fly.toml dev writes sets force_https.
#
# It cannot be destroyed, only set to something else: removing the records
# above leaves this as it is, which is right for the zone either way.
resource "cloudflare_zone_setting" "ssl" {
  zone_id    = "` + zone.ID + `"
  setting_id = "ssl"
  value      = "strict"
}

# Waits until Fly agrees the certificate is live, so an apply that finishes
# means the address actually works rather than that the records exist.
resource "fly_cert_validation" "app" {
  cert_id = fly_cert.app.id
  validation_dependencies = [
    cloudflare_dns_record.app.id,
    cloudflare_dns_record.ownership.id,
  ]
}

output "address" {
  value = "https://` + host + `/"
}
`
}
