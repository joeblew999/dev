// The key a release is signed with: made once, kept in fnox, its public half
// committed for consumers to pin. Nothing here builds or publishes anything.
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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeblew999/dev/internal/fnox"
	"github.com/joeblew999/dev/internal/secrets"
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
