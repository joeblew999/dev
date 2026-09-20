package fly

import (
	"bytes"
	"errors"
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
	old := fnox.Run
	fnox.Run = func(u fnox.Under) (string, error) {
		if out, handled := answer(u.Args); handled {
			if u.Out != nil {
				io.WriteString(u.Out, out)
			}
			return out, nil
		}
		got = append([]string{"in:" + u.Dir}, u.Args...)
		if u.Stdin != nil {
			b, _ := io.ReadAll(u.Stdin)
			got = append(got, "stdin:"+string(b))
		}
		return "", nil
	}
	t.Cleanup(func() { fnox.Run = old })
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
				// Asked only once something has already gone wrong, so the
				// happy path never pays for it.
				"flyctl auth whoami",
			},
			wantErr: "cannot be seen from here",
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
			old := fnox.Run
			fnox.Run = func(u fnox.Under) (string, error) {
				out, err := say(strings.Join(u.Args, " "))
				if u.Out != nil {
					io.WriteString(u.Out, out)
				}
				return out, err
			}
			t.Cleanup(func() { fnox.Run = old })
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

// What flyctl printed decides whether an app is there, not whether it exited
// zero and not what its prose says. This file has been wrong twice about
// flyctl — about which stream it answers on, and about an empty list meaning
// absence — so "the app is there" is taken only from an object naming itself.
func TestExistenceIsDecidedByTheJSONNotTheProse(t *testing.T) {
	for name, tc := range map[string]struct {
		said     string
		exitedOK bool
		want     string // "exists", "absent" or "error"
	}{
		"an object naming the app": {
			said: `{"Name":"acme-site","Hostname":"acme-site.fly.dev"}`, exitedOK: true, want: "exists",
		},
		// flyctl has exited non-zero while still printing the app before now;
		// the object is the evidence, so this is still yes.
		"the object, but a non-zero exit": {
			said: `{"Name":"acme-site"}`, exitedOK: false, want: "exists",
		},
		"the sentence for a missing app": {
			said: `Error: failed to get app: Could not find App "acme-site"`, exitedOK: false, want: "absent",
		},
		// Not absence: this is a credentials problem wearing a failure, and
		// treating it as absence is what tried to create a live app.
		"unauthorized": {
			said: "Error: unauthorized", exitedOK: false, want: "error",
		},
		// An empty array is what a scoped token returns for a list. It is not
		// an app, so it is not evidence of one.
		"an empty list": {
			said: "[]", exitedOK: false, want: "error",
		},
	} {
		t.Run(name, func(t *testing.T) {
			old := fnox.Run
			fnox.Run = func(fnox.Under) (string, error) {
				if tc.exitedOK {
					return tc.said, nil
				}
				return tc.said, fmt.Errorf("exit status 1")
			}
			t.Cleanup(func() { fnox.Run = old })

			err := appStatus("acme-site")
			got := "error"
			switch {
			case err == nil:
				got = "exists"
			case errors.Is(err, errNoSuchApp):
				got = "absent"
			}
			if got != tc.want {
				t.Errorf("appStatus said %q (%v); want %q", got, err, tc.want)
			}
		})
	}
}
