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
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/gitrepo"
	"github.com/joeblew999/dev/internal/secrets"
)

// The signing key. One long-lived Ed25519 key signs every release, local or
// CI, so a consumer pins one public key: mise's `pubkey` on the tool. The
// secret half lives in fnox for a developer and in the repo's Actions secret
// for CI, under the same name; the public half is committed as packslip.pub.
const (
	SigningKeyEnv = "PACKSLIP_SIGNING_KEY"
	pubFile       = "packslip.pub"
	// The binaries a release drives.
	GoreleaserBin = "goreleaser"
	PackslipBin   = "packslip"
)

// DistDir is where goreleaser writes and packslip signs, under a dot for the
// reason stage.BinDir is: build output is not source. dev states it rather
// than reading it, because dev generates the goreleaser config for whatever
// directory holds the main.go and commits nothing to the repo.
const DistDir = ".dist"

// release is one command directory as goreleaser and packslip see it.
type release struct {
	dir, name, slug string
	bins            []string // every binary the archives hold
	skills          []string // packslip resources, one per skills/<name>
	config          string   // the goreleaser config to use
	cleanup         func()
}

func newRelease(dir, name string) (*release, error) {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	s, err := slug()
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = s[strings.LastIndex(s, "/")+1:]
	}
	r := &release{dir: dir, name: name, slug: s, bins: []string{name}, cleanup: func() {}}
	entries, _ := os.ReadDir("skills")
	for _, e := range entries {
		if e.IsDir() {
			r.skills = append(r.skills, fmt.Sprintf("skill/%s=repo:skills/%s", e.Name(), e.Name()))
		}
	}
	// goreleaser is about to write DistDir; git should already be ignoring it.
	if err := cli.Ignore(".", DistDir); err != nil {
		return nil, err
	}
	r.config = ".goreleaser.yml"
	if data, err := os.ReadFile(r.config); err == nil {
		// A repo with its own config may build more than one binary; the
		// manifest must name every one, or mise exposes only the first.
		if bins := binaries(data); len(bins) > 0 {
			r.bins = bins
		}
		// dev looks for the artifacts under DistDir; a config that sends
		// goreleaser somewhere else signs nothing and says so far too late.
		if !bytes.Contains(data, []byte("\ndist: "+DistDir)) && !bytes.HasPrefix(data, []byte("dist: "+DistDir)) {
			return nil, fmt.Errorf("%s does not set `dist: %s`; add that line (dev builds under a dot, so .gitignore and mise outputs agree)", r.config, DistDir)
		}
	} else {
		f, err := os.CreateTemp("", "goreleaser-*.yml")
		if err != nil {
			return nil, err
		}
		if _, err := f.WriteString(goreleaserConfig(name, dir, Pubkey("."))); err != nil {
			return nil, err
		}
		f.Close()
		r.config = f.Name()
		r.cleanup = func() { os.Remove(f.Name()) }
	}
	return r, nil
}

var binaryLine = regexp.MustCompile(`(?m)^\s*binary:\s*"?([^"\s]+)"?\s*$`)

// binaries is every `binary:` a goreleaser config names, in order.
func binaries(config []byte) []string {
	var out []string
	for _, m := range binaryLine.FindAllSubmatch(config, -1) {
		out = append(out, string(m[1]))
	}
	return out
}

// goreleaserConfig is the one every repo on this stack would otherwise copy:
// a static binary for the platforms developers and CI run, tar.gz archives,
// checksums, and the release created in the repo goreleaser runs in. A main
// with `var version string` gets the release's version and one with `var
// pubkey string` the public key consumers pin; a main without is unaffected,
// since the linker ignores -X for a symbol it cannot find.
func goreleaserConfig(name, dir, pubkey string) string {
	return fmt.Sprintf(`version: 2
project_name: %s
dist: `+DistDir+`
builds:
  - id: %s
    dir: %s
    main: .
    binary: %s
    env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    flags: [-trimpath, -buildvcs=false]
    ldflags: ["-s -w -buildid= -X main.version={{ .Version }} -X main.pubkey=%s"]
archives:
  - formats: [tar.gz]
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    wrap_in_directory: false
    files: []
checksum:
  name_template: checksums.txt
release:
  replace_existing_artifacts: true
changelog:
  sort: asc
  filters:
    exclude: ["^docs:", "^test:", "^ci:", "^chore:", "Merge pull request", "Merge branch"]
`, name, name, dir, name, pubkey)
}

// run streams a command's output; the caller sees goreleaser, packslip and gh
// exactly as if they had run them by hand.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func out(name string, args ...string) (string, error) {
	raw, err := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(raw)), err
}

// slug is owner/repo, from the origin remote.
func slug() (string, error) { return gitrepo.Slug(".") }

// create signs DistDir/*.tar.gz into DistDir/packslip.sigstore.json.
func (r *release) create(version, commit, tag, key string, noLog bool) error {
	matches, err := filepath.Glob(DistDir + "/*.tar.gz")
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("no %s/*.tar.gz to sign; goreleaser wrote its archives elsewhere or built nothing", DistDir)
	}
	return run(PackslipBin, r.createArgs(version, commit, tag, key, noLog, matches)...)
}

// createArgs is the packslip create command line. The download URL of every
// artifact is the GitHub Release's, said explicitly: packslip infers nothing
// from --source-repo, and a manifest without URLs is one mise cannot install
// from (found the hard way on v0.1.0 of this tool).
func (r *release) createArgs(version, commit, tag, key string, noLog bool, artifacts []string) []string {
	repo := "https://github.com/" + r.slug
	args := []string{"create",
		"--project", "github.com/" + r.slug,
		"--version", version,
		"--out", DistDir,
	}
	for _, b := range r.bins {
		args = append(args, "--bin", b)
	}
	args = append(args,
		"--source-repo", repo,
		"--commit", commit,
		"--tag", tag,
		"--url-base", repo+"/releases/download/"+tag+"/",
		"--notes-url", repo+"/releases/tag/"+tag,
	)
	if key != "" {
		args = append(args, "--key", key)
	}
	if noLog {
		args = append(args, "--no-log")
	}
	for _, s := range r.skills {
		args = append(args, "--resource", s)
	}
	return append(args, artifacts...)
}

// snapshot builds the artifacts, signs the manifest with a throwaway key and
// verifies it, then shows it. Nothing is tagged or uploaded.
func (r *release) snapshot() error {
	defer r.cleanup()
	if err := run(GoreleaserBin, "release", "--snapshot", "--clean", "--config", r.config); err != nil {
		return err
	}
	describe, err := out("git", "describe", "--tags", "--always")
	if err != nil {
		return err
	}
	version, tag := snapshotVersion(describe)
	commit, err := out("git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	key, err := keygen("packslip-snapshot.key")
	if err != nil {
		return err
	}
	if err := r.create(version, commit, tag, key, true); err != nil {
		return err
	}
	// packslip writes the public key as "<id> <base64>"; verify wants the base64 alone.
	pub, err := out("tail", "-1", strings.TrimSuffix(key, filepath.Ext(key))+".pub")
	if err != nil {
		return err
	}
	if i := strings.LastIndex(pub, " "); i >= 0 {
		pub = pub[i+1:]
	}
	pubFile := key + ".b64"
	if err := os.WriteFile(pubFile, []byte(pub+"\n"), 0o600); err != nil {
		return err
	}
	matches, _ := filepath.Glob(DistDir + "/*.tar.gz")
	if err := run(PackslipBin, "verify", DistDir+"/packslip.sigstore.json", "--pubkey", pubFile, "--allow-unlogged", "--artifact", matches[0]); err != nil {
		return err
	}
	return run(PackslipBin, "show", DistDir+"/packslip.sigstore.json")
}

var semverStart = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// snapshotVersion turns what git describe said into the semver packslip
// wants. After a tag it is that tag with git's own prerelease suffix; in a
// repo with no tag yet, where describe is a bare commit, it is 0.0.0-<commit>.
func snapshotVersion(describe string) (version, tag string) {
	version = strings.TrimPrefix(describe, "v")
	if !semverStart.MatchString(version) {
		version = "0.0.0-" + describe
	}
	return version, "v" + version
}

// publish tags, pushes the tag, builds, signs and uploads: one path, run by
// a developer or by the release workflow on demand with a version. The key
// comes from fnox or, in CI, from the Actions secret; never a throwaway one,
// since consumers pin its public half. Nothing runs on a tag push, so a
// release is published exactly once.
func (r *release) publish(version string) error {
	defer r.cleanup()
	if version == "" {
		return fmt.Errorf("give the version to release, vX.Y.Z")
	}
	tag := version
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	if err := majorFitsModule(tag); err != nil {
		return err
	}
	if status, err := out("git", "status", "--porcelain"); err != nil {
		return err
	} else if status != "" {
		return fmt.Errorf("working tree is dirty; commit first")
	}
	if err := run("git", "tag", tag); err != nil {
		return err
	}
	// Push only the tag: pushing main too can advance the remote past the tag
	// when local main is ahead, and gh then targets the wrong repo state.
	if err := run("git", "push", "origin", "refs/tags/"+tag); err != nil {
		return err
	}
	commit, err := out("git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	// goreleaser needs a token env even though gh authenticates from its own
	// keychain; reuse it when set, else ask gh for one.
	if os.Getenv("GITHUB_TOKEN") == "" {
		if token, err := out(secrets.GhBin, "auth", "token"); err == nil && token != "" {
			os.Setenv("GITHUB_TOKEN", token)
		}
	}
	notes, err := os.CreateTemp("", "release-notes-*.md")
	if err != nil {
		return err
	}
	defer os.Remove(notes.Name())
	fmt.Fprintln(notes, "Release "+tag)
	notes.Close()
	if err := run(GoreleaserBin, "release", "--clean", "--config", r.config, "--release-notes", notes.Name()); err != nil {
		return err
	}
	// A local release signs with the long-lived key from fnox, logged: mise
	// refuses a bundle with no log entry, and a throwaway key would need a
	// new pin on every release. (A snapshot is never installed, so only it
	// signs with a throwaway key, unlogged.)
	key, cleanup, err := signingKey()
	if err != nil {
		return err
	}
	defer cleanup()
	if key == "" {
		return fmt.Errorf("no signing key: make one with: dev release %s --keygen (into fnox as %s, its public half into %s)", r.dir, SigningKeyEnv, pubFile)
	}
	if err := r.create(strings.TrimPrefix(tag, "v"), commit, tag, key, false); err != nil {
		return err
	}
	// goreleaser created the release when it published; upload into it, or
	// create it when goreleaser ran without a token to do so itself.
	files := []string{DistDir + "/packslip.sigstore.json", DistDir + "/checksums.txt"}
	matches, _ := filepath.Glob(DistDir + "/*.tar.gz")
	files = append(files, matches...)
	if err := run(secrets.GhBin, append(append([]string{"release", "upload", tag}, files...), "--clobber")...); err != nil {
		if err := run(secrets.GhBin, append(append([]string{"release", "create", tag}, files...), "--title", tag, "--notes", "Release "+tag)...); err != nil {
			return err
		}
	}
	fmt.Printf("published %s %s: https://github.com/%s/releases/tag/%s\n", r.name, tag, r.slug, tag)
	return nil
}

// majorFitsModule refuses a tag the module path cannot carry.
//
// Go requires a module released at v2 or above to say so in its path:
// github.com/owner/thing/v3. A path without that suffix may only carry v0 and
// v1 tags, and the toolchain does not warn about a v3 tag — it refuses to
// parse the require line, in the consumer's repo, long after the tag is
// published and unfixable.
//
// This repo published v2.0.0 and v3.0.0 before anything checked, and found
// out when a consumer could not import them. Nothing here can unpublish a
// tag, so the check is before one is made.
func majorFitsModule(tag string) error {
	var major int
	if _, err := fmt.Sscanf(tag, "v%d.", &major); err != nil || major < 2 {
		return nil
	}
	path, err := modulePath()
	if err != nil {
		return err
	}
	return majorFits(tag, path)
}

// majorFits is the rule itself, given the path, so a test can ask it without
// a go.mod to read.
func majorFits(tag, path string) error {
	var major int
	if _, err := fmt.Sscanf(tag, "v%d.", &major); err != nil || major < 2 {
		return nil
	}
	want := fmt.Sprintf("/v%d", major)
	if strings.HasSuffix(path, want) {
		return nil
	}
	return fmt.Errorf("%s cannot be released as %s: a module path without %s may only carry v0 and v1 tags, and Go refuses the require line rather than warning.\nEither release it as v1.x, or rename the module to %s%s and every import of it",
		path, tag, want, path, want)
}

// modulePath is what this repo's go.mod calls itself.
func modulePath() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", fmt.Errorf("reading go.mod to check the version: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("go.mod names no module")
}
