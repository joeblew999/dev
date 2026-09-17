package release

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeblew999/dev/fnox"
	"github.com/joeblew999/dev/secrets"
)

func TestSigningKeyComesFromTheEnvironmentThenFnox(t *testing.T) {
	old := fnox.Get
	fnox.Get = func(name string) (string, error) { return "from-fnox", nil }
	t.Cleanup(func() { fnox.Get = old })
	t.Setenv(SigningKeyEnv, "")
	path, cleanup, err := signingKey()
	if err != nil || path == "" {
		t.Fatalf("no key from fnox: %q, %v", path, err)
	}
	data, _ := os.ReadFile(path)
	st, _ := os.Stat(path)
	cleanup()
	if string(data) != "from-fnox" || st.Mode().Perm() != 0o600 {
		t.Errorf("key file = %q mode %v", data, st.Mode().Perm())
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("cleanup left the key file behind")
	}
	t.Setenv(SigningKeyEnv, "from-ci")
	path, cleanup, _ = signingKey()
	data, _ = os.ReadFile(path)
	cleanup()
	if string(data) != "from-ci" {
		t.Errorf("the environment did not win: %q", data)
	}
	fnox.Get = func(string) (string, error) { return "", os.ErrNotExist }
	t.Setenv(SigningKeyEnv, "")
	if path, _, _ := signingKey(); path != "" {
		t.Errorf("a key appeared from nowhere: %q", path)
	}
}

// Keygen runs the real packslip keygen; fnox and gh are seams.
func TestKeygenStoresTheSecretAndPublishesThePublicHalf(t *testing.T) {
	t.Chdir(t.TempDir())
	var stored, ci map[string]string = map[string]string{}, map[string]string{}
	oldGet, oldSet, oldCI := fnox.Get, fnox.Set, secrets.CI
	fnox.Get = func(name string) (string, error) { return stored[name], nil }
	fnox.Set = func(name, value string) error { stored[name] = value; return nil }
	secrets.CI = func(name, value string) error { ci[name] = value; return nil }
	t.Cleanup(func() { fnox.Get, fnox.Set, secrets.CI = oldGet, oldSet, oldCI })

	var out bytes.Buffer
	if err := Keygen(&out, false); err != nil {
		t.Fatal(err)
	}
	if stored[SigningKeyEnv] == "" || ci[SigningKeyEnv] != stored[SigningKeyEnv] {
		t.Errorf("fnox has %d bytes, CI has %d bytes; want the same key in both", len(stored[SigningKeyEnv]), len(ci[SigningKeyEnv]))
	}
	pub := Pubkey(".")
	if pub == "" || strings.Contains(pub, " ") || !strings.Contains(out.String(), pub) {
		t.Errorf("public key line %q; output:\n%s", pub, out.String())
	}
	if strings.Contains(out.String(), stored[SigningKeyEnv]) {
		t.Error("the secret key was printed")
	}
	if _, err := os.Stat(filepath.Join(os.TempDir(), "packslip-new.key")); err == nil {
		t.Error("the temporary secret key was left behind")
	}
	if err := Keygen(&out, false); err == nil || !strings.Contains(err.Error(), "--rotate") {
		t.Errorf("a second keygen replaced the key consumers pin: %v", err)
	}
	before := stored[SigningKeyEnv]
	out.Reset()
	if err := Keygen(&out, true); err != nil {
		t.Fatal(err)
	}
	if stored[SigningKeyEnv] == before || ci[SigningKeyEnv] != stored[SigningKeyEnv] {
		t.Error("rotation did not replace the key everywhere")
	}
	if !strings.Contains(out.String(), "mise packslip forget") {
		t.Errorf("rotation did not say what consumers must do:\n%s", out.String())
	}
}
