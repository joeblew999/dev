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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/fnox"
	"github.com/joeblew999/dev/internal/gitignore"
	"github.com/joeblew999/dev/internal/gitrepo"
	"github.com/joeblew999/dev/internal/secrets"
)

const Usage = `dev release DIR [VERSION] [--snapshot] [--name NAME]
    publish a GitHub Release of the command in DIR, the same locally and in
    GitHub Actions: VERSION here (vX.Y.Z), the pushed tag there. Build every
    platform with goreleaser, sign the packslip manifest, upload. Signed with
    the key in fnox (PACKSLIP_SIGNING_KEY), which --keygen makes once, with its
    public half in packslip.pub for consumers to pin (mise: pubkey = "...").
    --snapshot builds, signs with a throwaway key and verifies, publishing
    nothing; check runs it. NAME is the binary's name; default the repo's.
    Every directory under skills/ ships as a skill.

Needs goreleaser, packslip and gh, and a clean tree to publish.
`

// Run is `dev release DIR [VERSION]`.
func Run(verb string, args []string, stdout, stderr io.Writer) error {
	fs := cli.Flags(verb, stderr)
	var snapshot, keygen, rotate cli.Bool
	fs.Var(&snapshot, "snapshot", "build, sign with a throwaway key and verify; publish nothing")
	fs.Var(&keygen, "keygen", "make the signing key: into fnox, its public half into packslip.pub and the repo's Actions secret")
	fs.Var(&rotate, "rotate", "with --keygen: replace the key that exists, and say what every consumer must do")
	name := fs.String("name", "", "the binary's name (default: the repo's)")
	if len(args) == 0 || args[0] == "" || strings.HasPrefix(args[0], "-") {
		return cli.Usagef("release: the command directory comes first")
	}
	dir := args[0]
	rest, err := cli.ParseInterleaved(fs, args[1:])
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
	r, err := newRelease(dir, *name)
	if err != nil {
		return err
	}
	if keygen {
		return Keygen(stdout, bool(rotate))
	}
	if snapshot {
		return r.snapshot()
	}
	return r.publish(version)
}

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
	GhBin         = "gh"
)

// signingKey writes the key to a file packslip can read and returns its path,
// "" when there is no key. The environment wins (CI); then fnox.
func signingKey() (path string, cleanup func(), err error) {
	seed := os.Getenv(SigningKeyEnv)
	if seed == "" {
		seed, _ = fnox.Get(SigningKeyEnv)
	}
	if seed == "" {
		return "", func() {}, nil
	}
	f, err := os.CreateTemp("", "packslip-*.key")
	if err != nil {
		return "", func() {}, err
	}
	if err := os.Chmod(f.Name(), 0o600); err != nil {
		return "", func() {}, err
	}
	if _, err := f.WriteString(seed); err != nil {
		return "", func() {}, err
	}
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

// Pubkey is the public key line consumers pin, from packslip.pub, "" without.
func Pubkey(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, pubFile))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Keygen makes the signing key once: the secret into fnox and the repo's
// Actions secret (through gh, on stdin), the public half into packslip.pub
// for consumers to pin. It refuses when fnox already has one, since every
// consumer pins that one's public half.
func Keygen(out io.Writer, rotate bool) error {
	had := Pubkey(".")
	if v, _ := fnox.Get(SigningKeyEnv); v != "" && !rotate {
		return fmt.Errorf("%s is already in fnox and consumers pin its public key (%s); to replace it: dev release . --keygen --rotate", SigningKeyEnv, pubFile)
	}
	tmp, err := keygen("packslip-new.key")
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	defer os.Remove(strings.TrimSuffix(tmp, filepath.Ext(tmp)) + ".pub")
	seed, err := os.ReadFile(tmp)
	if err != nil {
		return err
	}
	pub, err := os.ReadFile(strings.TrimSuffix(tmp, filepath.Ext(tmp)) + ".pub")
	if err != nil {
		return err
	}
	if err := fnox.Set(SigningKeyEnv, string(seed)); err != nil {
		return fmt.Errorf("storing %s in fnox: %w", SigningKeyEnv, err)
	}
	if err := os.WriteFile(pubFile, pub, 0o644); err != nil {
		return err
	}
	if err := secrets.CI(SigningKeyEnv, string(seed)); err != nil {
		return fmt.Errorf("the key is in fnox and %s is written, but not in the repo's Actions secrets: %w; retry with: dev secrets ci %s", pubFile, err, SigningKeyEnv)
	}
	fmt.Fprintf(out, "signing key made: %s in fnox and in this repo's Actions secrets; commit %s.\nConsumers pin its public key:\n  \"packslip:github.com/<owner>/<repo>\" = { version = \"X.Y.Z\", pubkey = \"%s\" }\n", SigningKeyEnv, pubFile, Pubkey("."))
	if rotate && had != "" {
		fmt.Fprintf(out, "rotated from %s. The key signs every repo you release, so in each: dev secrets ci %s, commit its new %s.\nEvery consumer, on every machine: the new pubkey in its pin, then once: mise packslip forget packslip:github.com/<owner>/<repo>\n", had[:12]+"...", SigningKeyEnv, pubFile)
	}
	return nil
}

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
	if err := gitignore.Ensure(".", DistDir); err != nil {
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

// keygen mints an ephemeral signing key under dir and returns its path. packslip
// writes the public half on the stem (foo.key -> foo.pub); a stale one refuses
// to be overwritten, so every spelling is removed first.
func keygen(name string) (string, error) {
	key := filepath.Join(os.TempDir(), name)
	os.Remove(key)
	os.Remove(key + ".pub")
	os.Remove(strings.TrimSuffix(key, filepath.Ext(key)) + ".pub")
	if err := run(PackslipBin, "keygen", "--out", key); err != nil {
		return "", err
	}
	return key, nil
}

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
		if token, err := out(GhBin, "auth", "token"); err == nil && token != "" {
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
	if err := run(GhBin, append(append([]string{"release", "upload", tag}, files...), "--clobber")...); err != nil {
		if err := run(GhBin, append(append([]string{"release", "create", tag}, files...), "--title", tag, "--notes", "Release "+tag)...); err != nil {
			return err
		}
	}
	fmt.Printf("published %s %s: https://github.com/%s/releases/tag/%s\n", r.name, tag, r.slug, tag)
	return nil
}
