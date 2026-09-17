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
	"errors"
	"flag"
	"io"
	"strings"

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

func Run(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	Flags(fs)
	if len(args) == 0 || args[0] == "" || strings.HasPrefix(args[0], "-") {
		return cli.Usagef("release: the command directory comes first")
	}
	dir := args[0]
	rest, err := cli.ParseInterleaved(fs, args[1:])
	if errors.Is(err, cli.ErrHelp) {
		return err
	}
	if err != nil {
		return cli.Usagef("release: %v", err)
	}
	if len(rest) > 1 {
		return cli.Usagef("release: at most one VERSION")
	}
	version := ""
	if len(rest) == 1 {
		version = rest[0]
	}
	r, err := newRelease(dir, cli.Value(fs, "name"))
	if err != nil {
		return err
	}
	if cli.Given(fs, "keygen") {
		return Keygen(stdout, cli.Given(fs, "rotate"))
	}
	if cli.Given(fs, "snapshot") {
		return r.snapshot()
	}
	return r.publish(version)
}
