package cloudflare

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeblew999/dev/internal/suffix"

	"github.com/BurntSushi/toml"

	"github.com/joeblew999/dev/internal/fnox"
)

func TestWorkerNameFollowsWranglerRules(t *testing.T) {
	var cfg wranglerConfig
	if _, err := toml.Decode(`
name = "app"
[env.tinygo]
main = "x.mjs"
[env.live]
name = "app-live"
`, &cfg); err != nil {
		t.Fatal(err)
	}
	for env, want := range map[string]string{"": "app", "tinygo": "app-tinygo", "live": "app-live", "other": "app-other"} {
		if got := workerName(cfg, env); got != want {
			t.Errorf("env %q: got %q, want %q", env, got, want)
		}
	}
}

// fakeAccount serves the subdomain endpoint and counts the calls.
func fakeAccount(t *testing.T, token, subdomain string) (calls *int) {
	t.Helper()
	n := 0
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"success":false,"errors":[{"message":"Invalid API token"}]}`)
			return
		}
		fmt.Fprintf(w, `{"success":true,"result":{"subdomain":%q}}`, subdomain)
	}))
	srv.Start()
	old := subdomainEndpoint
	subdomainEndpoint = srv.URL + "/accounts/%s/workers/subdomain"
	t.Cleanup(func() { subdomainEndpoint = old })
	return &n
}

func stubFnox(t *testing.T, values map[string]string) {
	// Every stub starts from nothing read. The credential memo is as
	// invisible to a stub as it is fast, and without this a test that
	// replaces fnox gets whatever an earlier test caused to be read — which
	// here meant a real token and a real request.
	forgetCredentials()
	t.Cleanup(forgetCredentials)

	t.Helper()
	old := fnox.Get
	fnox.Get = func(name string) (string, error) {
		v, ok := values[name]
		if !ok {
			return "", fmt.Errorf("fnox: %s not found", name)
		}
		return v, nil
	}
	t.Cleanup(func() { fnox.Get = old })
}

func TestURLReadsTheSubdomainOnceAndKeepsIt(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(ConfigFile, []byte("name = \"app\"\n[env.tinygo]\nmain = \"x.mjs\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := fakeAccount(t, "tok", "someone")
	stubFnox(t, map[string]string{"CLOUDFLARE_API_TOKEN": "tok", "CLOUDFLARE_ACCOUNT_ID": "acct"})

	got, err := URL(".", "", false)
	if err != nil || got != "https://app.someone.workers.dev" {
		t.Fatalf("worker: got %q, %v", got, err)
	}
	got, err = URL(".", "tinygo", false)
	if err != nil || got != "https://app-tinygo.someone.workers.dev" {
		t.Fatalf("tinygo: got %q, %v", got, err)
	}
	if *calls != 1 {
		t.Fatalf("API asked %d times, want once", *calls)
	}
	data, _ := os.ReadFile(localFile)
	if !strings.Contains(string(data), "CLOUDFLARE_WORKERS_SUBDOMAIN = \"someone\"") {
		t.Fatalf("mise.local.toml:\n%s", data)
	}
	if _, err := URL(".", "", true); err != nil || *calls != 2 {
		t.Fatalf("refresh: %v, calls %d", err, *calls)
	}
}

func TestURLNamesTheMissingCredential(t *testing.T) {
	t.Chdir(t.TempDir())
	os.WriteFile(ConfigFile, []byte("name = \"app\"\n"), 0o644)
	stubFnox(t, map[string]string{})
	_, err := URL(".", "", false)
	want := "CLOUDFLARE_API_TOKEN is not in fnox; store it with: fnox set -g CLOUDFLARE_API_TOKEN"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}

func TestURLReportsWhatCloudflareSaid(t *testing.T) {
	t.Chdir(t.TempDir())
	os.WriteFile(ConfigFile, []byte("name = \"app\"\n"), 0o644)
	fakeAccount(t, "right", "x")
	stubFnox(t, map[string]string{"CLOUDFLARE_API_TOKEN": "wrong", "CLOUDFLARE_ACCOUNT_ID": "acct"})
	_, err := URL(".", "", false)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403: Invalid API token") {
		t.Fatalf("got %v", err)
	}
}

func TestCreatedReportsOnlyIDsWrittenBack(t *testing.T) {
	before := []byte(`name = "app"
kv_namespaces = [{ binding = "A" }, { binding = "B", id = "old" }]
[env.tinygo]
kv_namespaces = [{ binding = "A" }]
`)
	after := []byte(`name = "app"
kv_namespaces = [{ binding = "A", id = "new1" }, { binding = "B", id = "old" }]
[env.tinygo]
kv_namespaces = [{ binding = "A", id = "new2" }]
`)
	got, err := created(before, after)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"kv_namespaces A id=new1", "kv_namespaces A id=new2 (env tinygo)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
	if same, _ := created(before, before); len(same) != 0 {
		t.Fatalf("unchanged config reported %q", same)
	}
}

func TestCheckJudgesStatusAndBody(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			fmt.Fprint(w, "<h1>Pick a model</h1>")
		case "/down":
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, "error code: 1042")
		}
	}))
	srv.Start()
	if code, _, err := check(srv.URL+"/ok", "Pick a model"); err != nil || code != 200 {
		t.Fatalf("ok: %d %v", code, err)
	}
	if _, _, err := check(srv.URL+"/ok", "Nope"); err == nil || !strings.Contains(err.Error(), `does not contain "Nope"`) {
		t.Fatalf("missing text: %v", err)
	}
	if _, _, err := check(srv.URL+"/down", ""); err == nil || !strings.Contains(err.Error(), "answered 502: error code: 1042") {
		t.Fatalf("down: %v", err)
	}
}

func TestWaitReadyReadsTheLog(t *testing.T) {
	oldSleep := sleep
	sleep = func(time.Duration) {}
	t.Cleanup(func() { sleep = oldSleep })
	log := filepath.Join(t.TempDir(), "dev.log")
	os.WriteFile(log, []byte("Starting local server...\nReady on http://127.0.0.1:8787\n"), 0o644)
	if err := waitReady(log, time.Minute); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(log, []byte("ERROR: build failed\n"), 0o644)
	if err := waitReady(log, time.Minute); err == nil {
		t.Fatal("an ERROR line was not reported")
	}
	os.WriteFile(log, []byte("still starting\n"), 0o644)
	if err := waitReady(log, 0); err == nil || !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("timeout: %v", err)
	}
	// The one failure dev causes itself, so it gets a sentence rather than a
	// log to read: the scaffold writes today's compatibility_date, and the
	// workerd inside a pinned wrangler only understands dates up to its own
	// release. The Worker deploys and will not run locally, which makes
	// `dev smoke` fail on a config `dev deploy --to cloudflare` just wrote.
	os.WriteFile(log, []byte(`✘ [ERROR] service core:user:x: This Worker requires compatibility date "2026-09-20", `+tooNew+` "2026-05-15".`+"\n"), 0o644)
	err := waitReady(log, time.Minute)
	if err == nil || !strings.Contains(err.Error(), "compatibility_date") {
		t.Fatalf("a runtime older than the config said %v; want it named", err)
	}
	if !strings.Contains(err.Error(), "mise.toml") {
		t.Errorf("the error does not say what to change: %v", err)
	}
}

func TestSuffixGivesADeveloperTheirOwnApps(t *testing.T) {
	t.Setenv(suffix.Env, "alice")
	var cfg wranglerConfig
	toml.Decode("name = \"app\"\n[env.tinygo]\nmain = \"x.mjs\"\n[env.live]\nname = \"app-live\"\n", &cfg)
	for env, want := range map[string]string{"": "app-alice", "tinygo": "app-alice-tinygo", "live": "app-live-alice"} {
		if got := workerName(cfg, env); got != want {
			t.Errorf("env %q: got %q, want %q", env, got, want)
		}
	}
	got := string(withSuffix([]byte("name = \"app\"\nmain = \"x\"\n[env.live]\nname = \"app-live\"  # own\n")))
	if got != "name = \"app-alice\"\nmain = \"x\"\n[env.live]\nname = \"app-live-alice\"  # own\n" {
		t.Fatalf("copy:\n%s", got)
	}
	t.Setenv(suffix.Env, "")
	if string(withSuffix([]byte("name = \"app\"\n"))) != "name = \"app\"\n" {
		t.Fatal("no suffix must leave the config alone")
	}
}

// A credential is read once per run, because fnox.Get spawns a process and
// every request here wants two of them — eighteen spawns to read one
// account's domains, and most of the four seconds that took.
//
// The memo has to be as droppable as it is fast. Without that a test which
// replaces fnox gets whatever an earlier test caused to be read, and this one
// found out the expensive way: a stub with an empty store returned the real
// token and the code made a live request.
func TestACredentialIsReadOnceAndCanBeForgotten(t *testing.T) {
	reads := 0
	old := fnox.Get
	fnox.Get = func(name string) (string, error) {
		reads++
		return "value-" + name, nil
	}
	t.Cleanup(func() { fnox.Get = old; forgetCredentials() })
	forgetCredentials()

	for range 5 {
		if v, err := credential("SOME_TOKEN"); err != nil || v != "value-SOME_TOKEN" {
			t.Fatalf("credential = %q, %v", v, err)
		}
	}
	if reads != 1 {
		t.Errorf("fnox was asked %d times for one credential; once is the point", reads)
	}
	// A second name is its own read: a command that wants only the token
	// should not be made to fetch the account as well.
	if _, err := credential("OTHER"); err != nil {
		t.Fatal(err)
	}
	if reads != 2 {
		t.Errorf("reads = %d; a different name is a different read", reads)
	}
	// And forgetting means forgetting, which is what makes a stub work.
	forgetCredentials()
	if _, err := credential("SOME_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if reads != 3 {
		t.Errorf("reads = %d; after forgetting it should ask again", reads)
	}
}
