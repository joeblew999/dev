// The release verb: what it takes and what it runs. What a release is lives
// in release.go, and the key it is signed with in keys.go.
// Package release publishes a GitHub Release of one command directory:
// goreleaser builds the artifacts, packslip signs the manifest, gh uploads.
//
// It runs the same locally and in GitHub Actions, from the one mise task.
// `mise run release <version>` on a developer's machine is the day-to-day
// path; the release workflow runs the identical task on demand; neither
// replaces the other. In CI (GITHUB_REF_NAME set) the tag is the one pushed
// and packslip signs with the workflow's identity.
//
// Nothing is configured per repo. The binary is named after the repo, every
// directory under skills/ ships as a skill, and the goreleaser config is
// generated from the command directory unless the repo keeps its own.
package release

import (
	_ "embed"
	"flag"

	"github.com/joeblew999/dev/cli"
)

//go:embed usage.md
var Usage string

// Run is `dev release DIR [VERSION]`.
// Flags are what `dev release` takes. main.go hands this to cli, which renders
// the signature from it; Run calls it and reads the values back, so the manual
// cannot name a flag this does not register, or miss one it does.
func Flags(fs *flag.FlagSet) {
	fs.Var(new(cli.Bool), "snapshot", "build, sign with a throwaway key and verify; publish nothing")
	fs.Var(new(cli.Bool), "keygen", "make the signing key: into fnox, its public half into packslip.pub and the repo's Actions secret")
	fs.Var(new(cli.Bool), "rotate", "with --keygen: replace the key that exists, and say what every consumer must do")
	fs.String("name", "", "the binary's `NAME` (default: the repo's)")
}

// Run is `dev release DIR [VERSION]`. cli has parsed DIR and the flags, so
// what is left is the one positional this verb allows and what to do with it.
func Run(c cli.Call) error {
	if len(c.Args) > 1 {
		return c.Usagef("at most one VERSION")
	}
	version := ""
	if len(c.Args) == 1 {
		version = c.Args[0]
	}
	r, err := newRelease(c.Dir, c.Value("name"))
	if err != nil {
		return err
	}
	if c.Given("keygen") {
		return Keygen(c.Stdout, c.Given("rotate"))
	}
	if c.Given("snapshot") {
		return r.snapshot()
	}
	return r.publish(version)
}
