package fly

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeblew999/dev/internal/fnox"
	"github.com/joeblew999/dev/internal/suffix"
)

// capture replaces every way into fnox and records the command it would have
// run. Both doors, not one: stubbing Exec alone meant that the moment the code
// asked a question through Ask instead, these tests ran the real fnox against
// the real Fly account and passed while doing it.
func capture(t *testing.T) *[]string {
	t.Helper()
	var got []string
	// A question about an app answers that it is there, so a test about
	// deploying is about deploying and not about creating.
	answer := func(args []string) (string, bool) {
		if strings.Contains(strings.Join(args, " "), "status") {
			return `{"Name":"acme-site"}`, true
		}
		return "", false
	}
	oldExec, oldAsk := fnox.Exec, fnox.Ask
	fnox.Exec = func(dir string, stdin io.Reader, stdout io.Writer, args ...string) error {
		if out, handled := answer(args); handled {
			io.WriteString(stdout, out)
			return nil
		}
		got = append([]string{"in:" + dir}, args...)
		if stdin != nil {
			b, _ := io.ReadAll(stdin)
			got = append(got, "stdin:"+string(b))
		}
		return nil
	}
	fnox.Ask = func(dir string, args ...string) (string, error) {
		if out, handled := answer(args); handled {
			return out, nil
		}
		got = append([]string{"in:" + dir}, args...)
		return "", nil
	}
	t.Cleanup(func() { fnox.Exec, fnox.Ask = oldExec, oldAsk })
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
	if err := NoEnv(dir, "staging"); err == nil || !strings.Contains(err.Error(), "second directory") {
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

// Whether an app is there has three answers, not two, and the third is the
// one that sent a live deploy into `apps create`.
//
// It used to be inferred from `flyctl apps list`: not in the list meant not
// there. A token scoped with `flyctl tokens create org` — which this package
// recommended — returns an empty list while still resolving an org and
// validating a name, so a deployed app read as missing, creating it failed on
// Fly's global name check, and the developer got a validation error with
// nothing to do about it.
func TestDeployAsksAboutTheAppAndHandlesEachAnswer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  string // what `flyctl status` says; "" means it succeeds
		create  string // what `flyctl apps create` says; "" means it succeeds
		want    []string
		wantErr string
	}{
		{
			name:   "the app is there, so it is deployed and not created",
			status: "",
			want: []string{
				"flyctl status --app acme-site-probe --json",
				"flyctl deploy --config cmd/site/fly.toml --app acme-site-probe .",
			},
		},
		{
			name:   "no such app, so it is created first",
			status: `Could not find App "acme-site-probe"`,
			want: []string{
				"flyctl status --app acme-site-probe --json",
				"flyctl apps create acme-site-probe --org acme",
				"flyctl deploy --config cmd/site/fly.toml --app acme-site-probe .",
			},
		},
		{
			name:   "the name is taken by an app these credentials cannot see",
			status: `Could not find App "acme-site-probe"`,
			create: "Validation failed: Name has already been taken",
			want: []string{
				"flyctl status --app acme-site-probe --json",
				"flyctl apps create acme-site-probe --org acme",
			},
			wantErr: "cannot see it",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := appDir(t)
			var ran []string
			// Both doors into fnox are stubbed, into one recorder. Stubbing
			// only Exec let this test run the real fnox the moment the code
			// moved a call to Ask, and pass while doing it.
			say := func(line string) (string, error) {
				ran = append(ran, line)
				switch {
				case strings.Contains(line, "status") && tc.status != "":
					return tc.status, fmt.Errorf("exit status 1")
				case strings.Contains(line, "apps create") && tc.create != "":
					return tc.create, fmt.Errorf("exit status 1")
				}
				return "", nil
			}
			oldExec, oldAsk := fnox.Exec, fnox.Ask
			fnox.Exec = func(_ string, _ io.Reader, stdout io.Writer, args ...string) error {
				out, err := say(strings.Join(args, " "))
				io.WriteString(stdout, out)
				return err
			}
			fnox.Ask = func(_ string, args ...string) (string, error) {
				return say(strings.Join(args, " "))
			}
			t.Cleanup(func() { fnox.Exec, fnox.Ask = oldExec, oldAsk })
			oldLook := lookPath
			lookPath = func(string) (string, error) { return "/x/flyctl", nil }
			t.Cleanup(func() { lookPath = oldLook })
			t.Setenv(suffix.Env, "probe")
			t.Setenv("FLY_ORG", "acme")

			err := Deploy(io.Discard, dir, nil)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("deploy failed: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatal("deploy succeeded; want it to refuse")
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error is %q; want it to say %q", err, tc.wantErr)
			}
			if got := strings.Join(ran, "; "); got != strings.Join(tc.want, "; ") {
				t.Errorf("ran %q\nwant %q", got, strings.Join(tc.want, "; "))
			}
		})
	}
}
