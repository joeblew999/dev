package fly

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeblew999/dev/fnox"
	"github.com/joeblew999/dev/internal/suffix"
)

// capture replaces fnox.Exec and records the command it would have run.
func capture(t *testing.T) *[]string {
	t.Helper()
	var got []string
	old := fnox.Exec
	fnox.Exec = func(dir string, stdin io.Reader, stdout io.Writer, args ...string) error {
		if strings.Join(args, " ") == "flyctl apps list --json" {
			io.WriteString(stdout, `[{"Name":"acme-site"}]`)
			return nil
		}
		got = append([]string{"in:" + dir}, args...)
		if stdin != nil {
			b, _ := io.ReadAll(stdin)
			got = append(got, "stdin:"+string(b))
		}
		return nil
	}
	t.Cleanup(func() { fnox.Exec = old })
	oldLook := lookPath
	lookPath = func(string) (string, error) { return "/x/flyctl", nil }
	t.Cleanup(func() { lookPath = oldLook })
	return &got
}

func appDir(t *testing.T) string {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("cmd/site", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("cmd/site", ConfigFile), []byte("app = \"acme-site\"\nprimary_region = \"lhr\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return "cmd/site"
}

func TestDeployRunsFromTheRootWithTheConfigBesideTheCode(t *testing.T) {
	got := capture(t)
	dir := appDir(t)
	if err := Deploy(io.Discard, dir, []string{"--ha=false"}); err != nil {
		t.Fatal(err)
	}
	want := "in:. flyctl deploy --config cmd/site/fly.toml --ha=false ."
	if strings.Join(*got, " ") != want {
		t.Errorf("ran %q, want %q", strings.Join(*got, " "), want)
	}
}

func TestSuffixGivesADeveloperTheirOwnApp(t *testing.T) {
	got := capture(t)
	dir := appDir(t)
	t.Setenv(suffix.Env, "alice")
	u, err := URL(dir)
	if err != nil || u != "https://acme-site-alice.fly.dev" {
		t.Fatalf("URL = %q, %v", u, err)
	}
	if err := Deploy(io.Discard, dir, nil); err != nil {
		t.Fatal(err)
	}
	if s := strings.Join(*got, " "); !strings.Contains(s, "--app acme-site-alice") {
		t.Errorf("a suffixed deploy did not name the app: %q", s)
	}
}

func TestSecretsGoOnStdinNeverAsArguments(t *testing.T) {
	got := capture(t)
	dir := appDir(t)
	if err := PutSecret(dir, "API_KEY", "s3cret"); err != nil {
		t.Fatal(err)
	}
	s := strings.Join(*got, " ")
	if !strings.HasPrefix(s, "in:. flyctl secrets import --app acme-site stdin:API_KEY=s3cret\n") {
		t.Errorf("ran %q", s)
	}
	if strings.Contains(strings.TrimSuffix(s, "stdin:API_KEY=s3cret\n"), "s3cret") {
		t.Error("the value appeared as an argument")
	}
}

func TestErrorsNameTheirFix(t *testing.T) {
	dir := appDir(t)
	old := lookPath
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPath = old })
	err := Deploy(io.Discard, dir, nil)
	if err == nil || !strings.Contains(err.Error(), `flyctl = "latest"`) {
		t.Errorf("missing flyctl: %v; want the mise.toml line", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte("primary_region = \"lhr\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := App(dir); err == nil || !strings.Contains(err.Error(), `app = "<name>"`) {
		t.Errorf("no app: %v; want the line to add", err)
	}
	if err := noEnv(dir, "staging"); err == nil || !strings.Contains(err.Error(), "second directory") {
		t.Errorf("--env on Fly: %v; want the way to have two apps", err)
	}
}

func TestDestroyAsksThenRunsFlyctl(t *testing.T) {
	got := capture(t)
	dir := appDir(t)
	var out bytes.Buffer
	if err := Destroy(strings.NewReader("no\n"), &out, dir, "", false); err == nil {
		t.Error("a refusal destroyed the app")
	}
	if err := Destroy(nil, &out, dir, "", true); err != nil {
		t.Fatal(err)
	}
	if s := strings.Join(*got, " "); !strings.HasSuffix(s, "flyctl apps destroy acme-site --yes") {
		t.Errorf("ran %q", s)
	}
}

// An app the account lacks is created before the deploy; one it has is not.
func TestDeployCreatesAMissingApp(t *testing.T) {
	dir := appDir(t)
	var ran []string
	old := fnox.Exec
	fnox.Exec = func(_ string, _ io.Reader, stdout io.Writer, args ...string) error {
		ran = append(ran, strings.Join(args, " "))
		if strings.HasPrefix(strings.Join(args, " "), "flyctl apps list") {
			io.WriteString(stdout, "[]")
		}
		return nil
	}
	t.Cleanup(func() { fnox.Exec = old })
	oldLook := lookPath
	lookPath = func(string) (string, error) { return "/x/flyctl", nil }
	t.Cleanup(func() { lookPath = oldLook })
	t.Setenv(suffix.Env, "probe")
	t.Setenv("FLY_ORG", "acme")
	if err := Deploy(io.Discard, dir, nil); err != nil {
		t.Fatal(err)
	}
	want := "flyctl apps list --json; flyctl apps create acme-site-probe --org acme; flyctl deploy --config cmd/site/fly.toml --app acme-site-probe ."
	if got := strings.Join(ran, "; "); got != want {
		t.Errorf("ran %q\nwant %q", got, want)
	}
}
