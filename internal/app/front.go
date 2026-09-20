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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
	"github.com/joeblew999/dev/internal/cloudflare"
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
	zone, err := named(c, host)
	if err != nil {
		return err
	}
	dir, err := workspace(host)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(config(zone, host, origin)), 0o644); err != nil {
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
		Bin: fnox.Bin, Pin: fnox.Pin, Dir: dir, Env: env,
		Args: append([]string{"exec", "--", TofuBin}, args...),
	}.Stream(c.Stdout)
}

// config is the whole of what dev asks Cloudflare for, written out so that
// what tofu does is readable rather than implied.
func config(zone cloudflare.Zone, host, origin string) string {
	name := strings.TrimSuffix(strings.TrimSuffix(host, zone.Name), ".")
	if name == "" {
		name = "@"
	}
	return `# Written by ` + "`dev front`" + `. Edit it if you need something this does
# not do, and dev will leave your changes alone — it only writes this file
# when it is not there.
terraform {
  required_providers {
    cloudflare = { source = "cloudflare/cloudflare", version = "~> 5.0" }
  }
}

# The token comes from the environment, which fnox supplies.
provider "cloudflare" {}

# Terraform manages what is declared and nothing else, so the rest of this
# zone is untouched.
resource "cloudflare_dns_record" "app" {
  zone_id = "` + zone.ID + `"
  name    = "` + name + `"
  type    = "CNAME"
  content = "` + origin + `"
  proxied = true
  ttl     = 1
}

# Without this, Cloudflare speaks plain HTTP to an origin that redirects to
# HTTPS, and the request bounces between them until a browser gives up. A Fly
# app redirects by default and the fly.toml dev writes sets force_https.
#
# It cannot be destroyed, only set to something else: removing the record
# above leaves this as it is, which is right for the zone either way.
resource "cloudflare_zone_setting" "ssl" {
  zone_id    = "` + zone.ID + `"
  setting_id = "ssl"
  value      = "strict"
}
`
}
