package app

import (
	"flag"
	"io"

	"github.com/joeblew999/dev/cli"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTargetIsReadFromTheDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, d := range []string{"cf", "fl", "both", "none"} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"cf/wrangler.toml", "fl/fly.toml", "both/wrangler.toml", "both/fly.toml"} {
		if err := os.WriteFile(f, []byte("name = \"x\"\napp = \"x\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := Target("cf"); err != nil || got != "cloudflare" {
		t.Errorf("cf: %q, %v", got, err)
	}
	if got, err := Target("fl"); err != nil || got != "fly" {
		t.Errorf("fl: %q, %v", got, err)
	}
	if _, err := Target("both"); err == nil || !strings.Contains(err.Error(), "split it in two") {
		t.Errorf("both: %v", err)
	}
	if _, err := Target("none"); err == nil || !strings.Contains(err.Error(), "add one beside its main.go") {
		t.Errorf("none: %v", err)
	}
	if _, err := Target(filepath.Join("none", "missing")); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("missing: %v", err)
	}
}

func TestNameFollowsTheTarget(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("fl", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("fl/fly.toml", []byte("app = \"acme\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := Name("fl", ""); err != nil || got != "acme" {
		t.Errorf("Name = %q, %v", got, err)
	}
}

// Adding a cloud has to be adding an entry to clouds and nothing else. That
// was true of the verbs and was not true of Name and PutSecret, which still
// dispatched with `if target == "fly"` — the switch this registry exists to
// have replaced, surviving in the two functions nobody looked at because they
// are called from secrets rather than from a verb.
//
// The test is not "does it work"; it is "is every cloud complete". A cloud
// added with a gap here fails at whichever call site reaches the nil first,
// which is a panic somewhere unrelated.
func TestEveryCloudIsWholeSoAddingOneIsOneEdit(t *testing.T) {
	if len(clouds) == 0 {
		t.Fatal("no clouds, so the dispatch answers nothing")
	}
	for name, c := range clouds {
		if c.ConfigFile == "" {
			t.Errorf("cloud %q names no config file, so Target can never choose it", name)
		}
		if c.Needs == "" {
			t.Errorf("cloud %q names no CLI, so a machine without it gets an exit status instead of a sentence", name)
		}
		for what, missing := range map[string]bool{
			"Deployed":  c.Deployed == nil,
			"Deploy":    c.Deploy == nil,
			"Logs":      c.Logs == nil,
			"Delete":    c.Delete == nil,
			"Name":      c.Name == nil,
			"PutSecret": c.PutSecret == nil,
			"List":      c.List == nil,
			"Events":    c.Events == nil,
		} {
			if missing {
				t.Errorf("cloud %q has no %s, so whatever calls it panics", name, what)
			}
		}
		// Smoke is the one a target may decline, and declining is saying why:
		// a Fly app has no local runtime, and a reader needs that sentence
		// rather than a nil.
		if c.Smoke == nil && c.NoSmoke == "" {
			t.Errorf("cloud %q cannot smoke and does not say why", name)
		}
	}
}

// called is a verb's Call with the flags it registers, parsed from args.
func called(t *testing.T, dir string, flags func(*flag.FlagSet), args ...string) cli.Call {
	t.Helper()
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	flags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return cli.Call{Verb: "dev probe", Dir: dir, Flags: fs, Stdout: io.Discard, Stderr: io.Discard}
}

// `url` with neither --local nor --deployed has one sensible answer — the
// address the app really has — and it was written into one cloud's entry
// rather than into the rule, so the other cloud printed a blank line and
// exited 0. Driven by a made-up target, so it holds for a cloud added later
// as well as for the two here.
func TestTheAddressIsTheDeployedOneUnlessLocalWasAskedFor(t *testing.T) {
	asked := 0
	target := cloud{Deployed: func(cli.Call) (string, error) {
		asked++
		return "https://deployed/", nil
	}}
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"neither":            {nil, "https://deployed/"},
		"--local":            {[]string{"--local", "http://127.0.0.1:1"}, "http://127.0.0.1:1"},
		"--deployed":         {[]string{"--deployed"}, "https://deployed/"},
		"--deployed --local": {[]string{"--deployed", "--local", "http://127.0.0.1:1"}, "https://deployed/"},
	} {
		got, err := address(called(t, ".", URLFlags, tc.args...), target)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != tc.want {
			t.Errorf("%s: printed %q, want %q", name, got, tc.want)
		}
		if got == "" {
			t.Errorf("%s: printed nothing, which reads as a broken tool", name)
		}
	}
	if asked == 0 {
		t.Error("the target was never asked for its address")
	}
}

// --to names where a deploy goes, so it is either true or refused. It used to
// be read only when the directory had no config, so `--to cloudflare` on a
// Fly directory deployed to Fly and said nothing — and a typo did the same.
func TestToIsCheckedEvenWhenTheDirectoryAlreadyDeploysSomewhere(t *testing.T) {
	names := cli.SortedKeys(clouds)
	if len(names) < 2 {
		t.Skip("one cloud cannot disagree with another")
	}
	is, other := names[0], names[1]

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, clouds[is].ConfigFile), []byte("name = \"x\"\napp = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The cloud it is already on: nothing to say, and nothing refused.
	if err := toward(called(t, dir, DeployFlags, "--to", is), is); err != nil {
		t.Errorf("--to %s on a directory that deploys there: %v", is, err)
	}
	// A different cloud: refused, naming the file that decided and the cloud
	// it really deploys to.
	err := toward(called(t, dir, DeployFlags, "--to", other), other)
	if err == nil {
		t.Fatalf("--to %s deployed to %s without a word", other, is)
	}
	for _, want := range []string{other, is, clouds[is].ConfigFile} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q: %v", want, err)
		}
	}
	// A cloud nobody has heard of is a typo, whether or not the directory
	// already deploys somewhere.
	for _, where := range []string{dir, t.TempDir()} {
		if err := toward(called(t, where, DeployFlags, "--to", "clowdflare"), "clowdflare"); err == nil {
			t.Errorf("--to accepted a cloud that does not exist in %s", where)
		}
	}
}

// The CLI a target works through is named in the registry, so a machine
// without it is told which tool and what to add. Fly checked and Cloudflare
// did not, so the same failure was a sentence on one cloud and the exit
// status of a shim that could not resolve a version on the other.
func TestAMissingCLIIsNamedForEveryCloud(t *testing.T) {
	old := lookPath
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPath = old })
	for name, c := range clouds {
		err := reachable(c)
		if err == nil {
			t.Errorf("cloud %q ran with no %s installed", name, c.Needs)
			continue
		}
		for _, want := range []string{c.Needs, "mise.toml"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("cloud %q: %q does not mention %q", name, err, want)
			}
		}
	}
	// And with the tool there, nothing is in the way.
	lookPath = func(string) (string, error) { return "/x/bin", nil }
	for name, c := range clouds {
		if err := reachable(c); err != nil {
			t.Errorf("cloud %q refused with its CLI installed: %v", name, err)
		}
	}
}

// A flag a target cannot act on is refused, and refusing means saying why.
// --env was refused for Fly and --refresh was not: one Worker-only flag said
// so and the other was taken in silence and had no effect, which a reader has
// no way to tell from having worked.
func TestAnIgnoredFlagIsRefusedWithAReason(t *testing.T) {
	for name, c := range clouds {
		for flag, why := range c.Ignores {
			if why == "" {
				t.Errorf("cloud %q ignores --%s and does not say why", name, flag)
			}
			// The reason is for a person to act on, so it has to be a sentence
			// and not the flag's name again.
			if len(why) < 20 {
				t.Errorf("cloud %q: --%s is refused with %q, which tells nobody anything", name, flag, why)
			}
		}
	}
	// Fly is the case this came from: it has environments in neither sense,
	// and no workers.dev name to re-ask for.
	fly, ok := clouds["fly"]
	if !ok {
		t.Fatal("no fly cloud")
	}
	for _, flag := range []string{"env", "refresh"} {
		if _, refused := fly.Ignores[flag]; !refused {
			t.Errorf("a Fly app still accepts --%s, which it does nothing with", flag)
		}
	}
	// And Cloudflare acts on both, so it must not refuse them.
	if cf := clouds["cloudflare"]; len(cf.Ignores) > 0 {
		t.Errorf("cloudflare refuses %v; it is the cloud these flags are for", cli.SortedKeys(cf.Ignores))
	}
}

// The config file that names a target is the registry's, not something Target
// spells out for itself. It used to name both files in one expression, so a
// third cloud was an entry in clouds and an edit in Target — and the edit is
// the one nobody would think to make, because everything else about adding a
// cloud happens in the one place.
func TestTargetChoosesByTheRegistrysConfigFiles(t *testing.T) {
	for name, c := range clouds {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, c.ConfigFile), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := Target(dir)
		if err != nil || got != name {
			t.Errorf("a directory holding %s resolved to %q (%v); want %q", c.ConfigFile, got, err, name)
		}
	}
	// Every config file is named when there is none, so a reader is told what
	// would make the directory deployable rather than only that it is not.
	empty := t.TempDir()
	_, err := Target(empty)
	if err == nil {
		t.Fatal("an empty directory resolved to a cloud")
	}
	for _, file := range configFiles() {
		if !strings.Contains(err.Error(), file) {
			t.Errorf("the error does not mention %s: %v", file, err)
		}
	}
	// Two is refused, and says which two.
	both := t.TempDir()
	for _, c := range clouds {
		if err := os.WriteFile(filepath.Join(both, c.ConfigFile), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Target(both); err == nil || !strings.Contains(err.Error(), "one cloud") {
		t.Errorf("a directory holding every config gave %v", err)
	}
}

// A directory with no config gets one when told which cloud, and the file it
// gets is the one whose presence names that target — so the next verb, and
// the next person, find a directory that deploys somewhere.
func TestScaffoldWritesTheFileThatNamesTheTarget(t *testing.T) {
	for name, c := range clouds {
		if c.Scaffold == nil {
			t.Errorf("cloud %q writes no config, so --to %s cannot work", name, name)
			continue
		}
		dir := t.TempDir()
		if _, err := Target(dir); err == nil {
			t.Fatal("an empty directory already resolved to a cloud")
		}
		var out strings.Builder
		if err := scaffold(&out, dir, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// The point of writing it: the directory now deploys there.
		got, err := Target(dir)
		if err != nil || got != name {
			t.Errorf("after scaffolding %s the directory resolves to %q (%v)", name, got, err)
		}
		// Named after the directory, as everything on this stack is.
		written, err := os.ReadFile(filepath.Join(dir, c.ConfigFile))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(written), filepath.Base(dir)) {
			t.Errorf("%s does not name the directory it is for:\n%s", c.ConfigFile, written)
		}
		// It says what wrote it, because a generated file that does not is a
		// file someone edits without knowing what will happen.
		if !strings.Contains(string(written), "dev deploy --to") {
			t.Errorf("%s does not say what wrote it:\n%s", c.ConfigFile, written)
		}
		if !strings.Contains(out.String(), c.ConfigFile) {
			t.Errorf("scaffolding said %q; it should name the file it wrote", out.String())
		}
	}
	// A cloud nobody has heard of is a typo, answered with the ones there are.
	if err := scaffold(io.Discard, t.TempDir(), "clowdflare"); err == nil {
		t.Error("--to accepted a cloud that does not exist")
	} else if !strings.Contains(err.Error(), "cloudflare") {
		t.Errorf("the error does not offer the name that was meant: %v", err)
	}
}

// A config dev writes has to be one dev can read. Each target parses its own
// file to answer what the app is called, so a scaffold that does not come
// back through that is worse than none: it deploys nothing and the failure
// lands somewhere else entirely.
//
// This is the round trip — write it, then ask the same cloud what the app in
// that directory is named.
func TestAScaffoldIsReadableByTheTargetThatWroteIt(t *testing.T) {
	for name, c := range clouds {
		dir := t.TempDir()
		if err := scaffold(io.Discard, dir, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got, err := c.Name(dir, "")
		if err != nil {
			t.Errorf("%s wrote a config it cannot read: %v", name, err)
			continue
		}
		if want := filepath.Base(dir); got != want {
			t.Errorf("%s: the config it wrote names %q; want %q", name, got, want)
		}
	}
}

// The settings that are the point of writing a config at all rather than an
// empty file. Each is one a project would otherwise discover it wanted after
// the incident that needed it.
func TestScaffoldsCarryTheSettingsWorthHaving(t *testing.T) {
	for name, want := range map[string][]string{
		// A Worker without observability is a black box, and it cannot be
		// turned on retroactively for the failure being investigated.
		"cloudflare": {"[observability]", "enabled = true", "head_sampling_rate", "compatibility_flags"},
		// Fly routes to a machine it believes healthy, so a service it does
		// not check is one it routes to before the service is ready. The
		// rest is what keeps an idle demo from billing.
		"fly": {"[[http_service.checks]]", "grace_period", "auto_stop_machines", "min_machines_running", "[[vm]]", "concurrency"},
	} {
		c, ok := clouds[name]
		if !ok {
			t.Fatalf("no cloud %q", name)
		}
		written := c.Scaffold(".", "probe")
		for _, line := range want {
			if !strings.Contains(written, line) {
				t.Errorf("%s's config does not set %s:\n%s", name, line, written)
			}
		}
		// Every setting says why it is there, or it is a line nobody dares
		// delete and nobody understands.
		if strings.Count(written, "#") < 5 {
			t.Errorf("%s's config explains too little of itself:\n%s", name, written)
		}
	}
}

// A target says how far back it can really see, because the two clouds do not
// keep the same amount and --since would otherwise be a promise one of them
// cannot meet. Cloudflare stores seven days of Workers Logs; Fly hands back
// whatever is still in flyctl's buffer, which is not a window at all.
func TestEveryTargetSaysHowFarBackItSees(t *testing.T) {
	for name, c := range clouds {
		if c.Keeps == "" {
			t.Errorf("cloud %q does not say what it keeps, so --since promises something nobody checked", name)
		}
	}
}

// The default is what "what just happened" means, and both bounds have to be
// bounded: Cloudflare keeps seven days and a query with no limit reads all of
// it, and Fly's stream has no end.
func TestTelemetryDefaultsAreBounded(t *testing.T) {
	got := Telemetry{}.Defaults()
	if got.Since <= 0 || got.Limit <= 0 {
		t.Fatalf("Defaults() = %+v; both have to be bounded", got)
	}
	// What a caller did say is kept.
	asked := Telemetry{Since: 3 * time.Minute, Limit: 7}.Defaults()
	if asked.Since != 3*time.Minute || asked.Limit != 7 {
		t.Errorf("Defaults() overrode what was asked for: %+v", asked)
	}
}

// The request is the one richer thing both clouds really keep, which is why
// it is the one that unifies. Everything else they record is one-sided —
// Cloudflare's CPU and wall time, Fly's region and machine — and a field that
// is always empty for one of them reads as missing data rather than as a
// difference between clouds.
func TestOnlyWhatBothCloudsKeepIsUnified(t *testing.T) {
	// Nothing recorded is nil, not a request to nowhere. Fly writes the HTTP
	// block on every line and fills it only when there was one, so without
	// this every deploy message would carry an empty request.
	if got := request("", "", "", 0); got != nil {
		t.Errorf("an empty request became %+v; want nil", got)
	}
	// Any one of the three is enough to be a request: a line may record the
	// status without the URL, or the method before anything came back.
	for name, r := range map[string]*Request{
		"method only": request("GET", "", "", 0),
		"url only":    request("", "https://x/", "", 0),
		"status only": request("", "", "", 500),
		"all three":   request("GET", "https://x/", "", 200),
		"id only":     request("", "", "req-1", 0),
	} {
		if r == nil {
			t.Errorf("%s was dropped as empty", name)
		}
	}
}
