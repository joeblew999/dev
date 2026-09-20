// Package app is what a command becomes once deployed, on whichever cloud its
// directory names: a wrangler.toml means Cloudflare Workers, a fly.toml means
// Fly. The verbs are the same for both; this package reads the directory and
// hands over to the target, so a task never says which cloud.
//
// Two things vary on their own here. Which cloud a directory deploys to is a
// property of the directory; what is being asked for is a property of the
// verb. Neither decides the other, so something has to hold the pair, and
// `clouds` is it: one entry per target, every operation that differs filled
// in, and one lookup reaching it.
//
// It lives below its callers rather than in any of them because more than one
// needs it — the verbs in main, and secrets, which pushes a secret through
// whichever CLI the directory implies and runs no verb at all. Putting the
// table in main would make it unreachable from secrets, since main imports
// secrets and not the other way about.
//
// The test of all this is that adding a third cloud is adding one entry and
// nothing else. A test holds every entry complete, because a gap would not
// fail where the cloud was added; it would fail later, as a nil, at whichever
// call site reached it first.
package app

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/cloudflare"
	"github.com/joeblew999/dev/internal/fly"
	"github.com/joeblew999/dev/internal/stage"
)

// Usage is what dev prints for these verbs. It is markdown in a file beside
// this one, not a string const: a Go raw string is backtick-delimited, so it
// can never hold the inline code that keeps a `<placeholder>` from reaching a
// markdown renderer as an HTML tag.

//go:embed usage.md
var Usage string

// The deploy verbs' flags. Both clouds register the same ones, checked verb by
// verb, with one exception: a Worker's smoke takes --path, --expect and
// --timeout and a Fly app's takes none, because only the Worker is run locally
// under workerd. The signature shows the Worker's, which is the larger set,
// and smoke's description says so.
//
// This is the one place a signature cannot be the whole truth: which cloud a
// verb is talking to is read from DIR, so it is not known until the verb runs.
//
// It once said here that `<verb> DIR --help` resolves the directory and prints
// that cloud's flags. It does not, and it cannot: cli registers a verb's flags
// from Verb.Flags, which is func(*flag.FlagSet) and never sees the directory,
// so help is the same for both clouds and lists --env and --refresh on a Fly
// app that refuses them. Changing that means changing a signature every
// command on the stack implements, for help text.
//
// So the flags are the union, and Ignores below carries the other half: a
// target says which of them it cannot act on, and asking for one is answered
// with why rather than with silence. That was the real fault — --env was
// refused and --refresh was accepted and did nothing, which reads as having
// worked.

// EnvFlag is the environment a deploy verb acts in. It is stage's, because the
// environment is a property of the build and not of the deploy; written out
// here as well, the two wordings drifted the moment either was edited.
func EnvFlag(fs *flag.FlagSet) { stage.EnvFlag(fs) }

// URLFlags are what `url` takes.
func URLFlags(fs *flag.FlagSet) {
	EnvFlag(fs)
	fs.Var(new(cli.Bool), "deployed", "the deployed app's URL; otherwise --local")
	fs.Var(new(cli.Bool), "refresh", "ask the API again instead of reading mise.local.toml")
	fs.String("local", "", "the `URL` to print when not --deployed")
}

// DeployFlags are what `deploy` takes.
// toFlag names the cloud for a directory that has none, so `deploy --to fly`
// writes the config and goes. It is only on deploy: every other verb acts on
// something already deployed, and there is nothing to choose.
func toFlag(fs *flag.FlagSet) {
	fs.String("to", "", "the `CLOUD` to deploy to, writing its config when the directory has none")
}

// LogsFlags are what logs takes. --json turns a stream into an answer that
// ends, and the two bounds are only meaningful with it.
func LogsFlags(fs *flag.FlagSet) {
	EnvFlag(fs)
	cli.JSONFlags(fs)
	fs.String("since", "1h", "how far back to read, with --json")
	fs.String("limit", "100", "at most this many events, with --json")
	fs.Var(new(cli.Bool), "raw", "carry each cloud's own record too: the headers, timings and everything the shared shape has nowhere to put")
}

func DeployFlags(fs *flag.FlagSet) {
	toFlag(fs)
	EnvFlag(fs)
	fs.String("wait", "", "the `PATH` to wait for a 200 on after deploying, e.g. /health")
}

// SmokeFlags are what `smoke` takes against a Worker; a Fly app takes none.
func SmokeFlags(fs *flag.FlagSet) {
	EnvFlag(fs)
	fs.String("path", "/", "the `P` to request")
	fs.String("expect", "", "the `TEXT` the body must contain")
	fs.Duration("timeout", 3*time.Minute, "how `LONG` wrangler dev may take to start")
}

// DeleteFlags are what `delete` takes.
func DeleteFlags(fs *flag.FlagSet) {
	EnvFlag(fs)
	fs.String("name", "", "the `APP` to remove (default: the one the config deploys env to)")
	fs.Var(new(cli.Bool), "yes", "remove without asking")
}

// WaitFlags are what `wait` takes.
func WaitFlags(fs *flag.FlagSet) {
	fs.Duration("timeout", 2*time.Minute, "how `LONG` to keep trying")
}

// Each verb is the work it does, and which cloud does it comes from the
// directory. cli has already parsed DIR and the flags, so these are the
// dispatch and nothing else.

func URLVerb(c cli.Call) error    { return to(c, "url") }
func DeployVerb(c cli.Call) error { return to(c, "deploy") }
func LogsVerb(c cli.Call) error   { return to(c, "logs") }
func SmokeVerb(c cli.Call) error  { return to(c, "smoke") }
func DeleteVerb(c cli.Call) error { return to(c, "delete") }
func ListVerb(c cli.Call) error   { return to(c, "list") }

// FrontingVerb takes a hostname rather than a directory: what stands in front
// of an app is a fact about a name, not about where the code is. An app may
// be fronted at a hostname that no directory here mentions, and a directory
// may be reachable at several.
func FrontingVerb(c cli.Call) error { return Fronting(c, c.Args[0]) }

// DomainsVerb takes nothing: it is about the account, not about a directory
// or a name.
func DomainsVerb(c cli.Call) error { return Domains(c) }

// FrontVerb puts Cloudflare in front of a host, or says what it would do.
func FrontVerb(c cli.Call) error { return Front(c, c.Args[0], c.Args[1]) }

// UnfrontVerb takes it away again.
func UnfrontVerb(c cli.Call) error { return Unfront(c, c.Args[0]) }

// FrontFlags are what front takes. The zone is named rather than inferred,
// and nothing happens without --apply.
func FrontFlags(fs *flag.FlagSet) {
	fs.String("zone", "", "the `ZONE` this is about, named rather than inferred from the host")
	fs.Var(new(cli.Bool), "apply", "make the changes; without it, only say what they would be")
}

// UnfrontFlags are what unfront takes.
func UnfrontFlags(fs *flag.FlagSet) {
	fs.String("zone", "", "the `ZONE` this is about")
	fs.Var(new(cli.Bool), "yes", "do not ask")
}

// FrontingFlags are what fronting takes.
func FrontingFlags(fs *flag.FlagSet) { cli.ReportFlags(fs) }

// WaitVerb is the one verb here that talks to no cloud: it polls a URL.
func WaitVerb(c cli.Call) error {
	d, _ := c.ValueAs("timeout", time.ParseDuration)
	return Wait(c.Stdout, c.Args[0], d)
}

// cloud is what only a target knows: how to reach it. Everything a verb does
// that is the same on both — print the address, wait for a path after
// deploying, refuse what a target cannot do — is in to() below, once.
//
// Both clouds used to carry a switch over the verbs, and the two switches
// said the same thing in two wordings: print the URL, deploy then wait, hand
// the rest straight on. A verb's meaning now lives where the verb does.
type cloud struct {
	// Scaffold is a conventional config for a directory that has none, named
	// after the directory. goreleaser's is the precedent: a config is made at
	// the moment it is needed rather than by an init verb, so a repo that
	// wants its own writes one and nobody else has to.
	//
	// Unlike goreleaser's, this one is written into the repo and committed.
	// It has to be: the file's presence is what names the target, so a
	// temporary one would mean the directory deployed nowhere the next time
	// anybody looked.
	Scaffold func(dir, name string) string

	// ConfigFile is the file whose presence in a directory names this target.
	// It is here rather than read straight from the two packages because
	// Target used to name both of them in one expression, so a third cloud
	// was an entry here and an edit there — and the edit there is the one
	// nobody would think to make.
	ConfigFile string

	// Ignores are the flags this target cannot act on, and why in words a
	// person can use. A flag a target does nothing with is refused rather
	// than accepted: --env was refused for Fly and --refresh was not, so one
	// Worker-only flag said so and the other was taken in silence and had no
	// effect, which reads as having worked.
	Ignores map[string]string

	Before   func(c cli.Call) error           // checked before any verb runs
	URL      func(c cli.Call) (string, error) // what `url` prints, flags and all
	Deployed func(c cli.Call) (string, error) // the deployed address, whatever --local says
	Deploy   func(c cli.Call) error
	Logs     func(c cli.Call) error
	Delete   func(c cli.Call) error
	Smoke    func(c cli.Call) error // nil when the target has no local runtime
	NoSmoke  string                 // and why, in words a person can act on

	// Events is what the app has been saying, bounded and structured — the
	// same question Logs answers by streaming, asked in a way that ends.
	// Logs is for a person watching; this is for everything else.
	Events func(c cli.Call, t Telemetry) ([]Event, error)

	// Keeps is how far back Events can really see, in words, because the two
	// clouds are not the same and pretending otherwise makes --since a lie.
	//
	// Cloudflare stores Workers Logs and answers a query over seven days of
	// them. Fly streams from its machines: `flyctl logs --no-tail` returns
	// what is in the buffer, which is recent and small and not a window at
	// all. Fly does keep seven days behind an HTTP API its own documentation
	// calls not officially documented for external use, so this does not use
	// it — and says so rather than quietly returning less than was asked for.
	Keeps string

	// List is what this account has deployed on this target. It is the half
	// of a lifecycle that was missing: dev could put an app in a cloud and
	// take it away again, and never say what was there — so "what did I
	// leave running" was a question only the cloud's own CLI could answer,
	// which is the moment somebody reaches past dev and stops getting the
	// suffix, the env and the account dev would have used.
	List func() ([]string, error)

	// Name and PutSecret take plain arguments rather than a Call, because
	// secrets reaches for them with no verb running. They are here all the
	// same: they were the last two places that dispatched with `if target ==
	// "fly"`, which is the switch this struct exists to have replaced.
	Name      func(dir, env string) (string, error)
	PutSecret func(dir, env, name, value string) error
}

// clouds is every target, by the name Target answers with.
var clouds = map[string]cloud{
	"cloudflare": {
		ConfigFile: cloudflare.ConfigFile,
		Scaffold:   cloudflare.Scaffold,
		URL: func(c cli.Call) (string, error) {
			return cloudflare.URL(c.Dir, c.Value("env"), c.Given("deployed"), c.Value("local"), c.Given("refresh"))
		},
		Deployed: func(c cli.Call) (string, error) {
			return cloudflare.URL(c.Dir, c.Value("env"), true, "", false)
		},
		Deploy: func(c cli.Call) error { return cloudflare.Deploy(c.Stdout, c.Dir, c.Value("env")) },
		Logs:   func(c cli.Call) error { return cloudflare.Logs(c.Dir, c.Value("env")) },
		Delete: func(c cli.Call) error {
			return cloudflare.Delete(c.Stdin, c.Stdout, c.Dir, c.Value("env"), c.Value("name"), c.Given("yes"))
		},
		Smoke: func(c cli.Call) error {
			d, _ := c.ValueAs("timeout", time.ParseDuration)
			return cloudflare.Smoke(c.Stdout, c.Dir, c.Value("env"), c.Value("path"), c.Value("expect"), d)
		},
		Name:      cloudflare.Name,
		PutSecret: cloudflare.PutSecret,
		List:      cloudflare.List,
		Keeps:     "seven days, for a Worker whose config enables observability",
		Events: func(c cli.Call, t Telemetry) ([]Event, error) {
			name, err := cloudflare.Name(c.Dir, c.Value("env"))
			if err != nil {
				return nil, err
			}
			said, err := cloudflare.Events(name, t.Since, t.Limit, t.Raw)
			if err != nil {
				return nil, err
			}
			return cli.Map(said, func(e cloudflare.Event) Event {
				return Event{At: e.At, Level: e.Level, Message: e.Message,
					From:    from(e.Type == cloudflare.WorkerLine),
					Request: request(e.Method, e.URL, e.ID, e.Status),
					Raw:     e.Raw}
			}), nil
		},
	},
	"fly": {
		ConfigFile: fly.ConfigFile,
		Scaffold:   fly.Scaffold,
		Ignores: map[string]string{
			"env":     "a Fly app has no environments: fly.toml deploys one app, and a second app is a second directory",
			"refresh": "--refresh re-asks the Workers API for a workers.dev name; a Fly app's address is its app name and is already exact",
		},
		URL: func(c cli.Call) (string, error) {
			// A Fly app has no local address to work out, so --local is
			// whatever the caller runs it on — and when they did not say, the
			// app has exactly one address and that is the answer.
			//
			// Returning the empty --local regardless meant `dev url DIR`
			// printed a blank line and exited 0, which reads as a broken tool
			// rather than as a question that was not asked properly.
			if local := c.Value("local"); local != "" && !c.Given("deployed") {
				return local, nil
			}
			return fly.URL(c.Dir)
		},
		Deployed: func(c cli.Call) (string, error) { return fly.URL(c.Dir) },
		Deploy:   func(c cli.Call) error { return fly.Deploy(c.Stdout, c.Dir, c.Args) },
		Logs:     func(c cli.Call) error { return fly.Logs(c.Dir) },
		Delete: func(c cli.Call) error {
			return fly.Destroy(c.Stdin, c.Stdout, c.Dir, c.Value("name"), c.Given("yes"))
		},
		NoSmoke: "smoke runs a Worker on local workerd; a Fly app has no local runtime here. dev check DIR tests it, and dev deploy DIR --wait PATH proves it online",
		// A Fly app's name is in its fly.toml and has no environments, so both
		// take the dir alone and ignore the env a Worker needs.
		List:  fly.List,
		Keeps: "only what flyctl still holds in its buffer, which is recent and not a window",
		Events: func(c cli.Call, t Telemetry) ([]Event, error) {
			said, err := fly.Events(c.Dir, t.Since, t.Limit, t.Raw)
			if err != nil {
				return nil, err
			}
			return cli.Map(said, func(e fly.Event) Event {
				return Event{At: e.At, Level: e.Level, Message: e.Message, Source: e.Source,
					From:    from(e.Provider == fly.AppLine),
					Request: request(e.Method, e.URL, e.ID, e.Status),
					Raw:     e.Raw}
			}), nil
		},
		Name:      func(dir, _ string) (string, error) { return fly.App(dir) },
		PutSecret: func(dir, _, name, value string) error { return fly.PutSecret(dir, name, value) },
	},
}

// to sends a verb to whichever cloud the directory deploys to, and does the
// part that is the same wherever it went.
func to(c cli.Call, verb string) error {
	// A directory with no config and a --to gets one and carries on, which is
	// the whole of "make it if it is not there": the config is written at the
	// moment something needs it, not by a separate verb nobody remembers.
	if want := c.Value("to"); want != "" && verb == "deploy" {
		if _, err := Target(c.Dir); err != nil {
			if err := scaffold(c.Stdout, c.Dir, want); err != nil {
				return err
			}
		}
	}
	t, err := cloudFor(c.Dir)
	if err != nil {
		return err
	}
	// A flag this target cannot act on is refused before anything runs, so
	// the answer is the same whichever verb was asked for.
	for _, name := range cli.SortedKeys(t.Ignores) {
		if c.Set(name) {
			return cli.Usagef("--%s: %s", name, t.Ignores[name])
		}
	}
	if t.Before != nil {
		if err := t.Before(c); err != nil {
			return err
		}
	}
	switch verb {
	case "url":
		u, err := t.URL(c)
		if err != nil {
			return err
		}
		fmt.Fprintln(c.Stdout, u)
		return nil
	case "deploy":
		if err := t.Deploy(c); err != nil {
			return err
		}
		// Deploying and then proving it answers is one thing a person does,
		// so it is one flag rather than a second command to remember.
		if c.Value("wait") == "" {
			return nil
		}
		u, err := t.Deployed(c)
		if err != nil {
			return err
		}
		return Wait(c.Stdout, u+c.Value("wait"), 2*time.Minute)
	case "logs":
		// A person watching gets the stream; everything else gets an answer
		// that ends. The flag that already means "for a reader that is not a
		// person" decides, as it does everywhere else here.
		if !c.WantsJSON() {
			return t.Logs(c)
		}
		since, _ := c.ValueAs("since", time.ParseDuration)
		limit, _ := c.ValueAs("limit", strconv.Atoi)
		// Asking for further back than the target keeps is answered, but not
		// silently: what comes back is whatever there was, and a reader who
		// is not told that reads an empty answer as an app that said nothing.
		if c.Set("since") && t.Keeps != "" {
			fmt.Fprintf(c.Stderr, "note: this target keeps %s\n", t.Keeps)
		}
		events, err := t.Events(c, Telemetry{Since: since, Limit: limit, Raw: c.Given("raw")}.Defaults())
		if err != nil {
			return err
		}
		return c.EmitJSON(events)
	case "delete":
		return t.Delete(c)
	case "smoke":
		if t.Smoke == nil {
			return fmt.Errorf("%s", t.NoSmoke)
		}
		return t.Smoke(c)
	case "list":
		names, err := t.List()
		if err != nil {
			return err
		}
		if len(names) == 0 {
			fmt.Fprintf(c.Stdout, "nothing deployed here\n")
			return nil
		}
		// The one this directory is, marked, because the question behind
		// this is usually "is mine up, and what else did I leave running".
		mine, _ := t.Name(c.Dir, c.Value("env"))
		for _, name := range names {
			if name == mine {
				fmt.Fprintf(c.Stdout, "* %s\n", name)
				continue
			}
			fmt.Fprintf(c.Stdout, "  %s\n", name)
		}
		return nil
	}
	// Unreachable: to() is only ever called with one of the verbs above, from
	// this file. It is here so that adding a verb without adding its case is a
	// message and not a silent success.
	return cli.Usagef("%v", cli.Unknown("deploy verb", verb,
		[]string{"url", "deploy", "logs", "delete", "smoke"}))
}

// Target names the cloud dir deploys to, "cloudflare" or "fly", from the
// config file it holds.
func Target(dir string) (string, error) {
	var found []string
	for _, name := range cli.SortedKeys(clouds) {
		if exists(filepath.Join(dir, clouds[name].ConfigFile)) {
			found = append(found, name)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return "", fmt.Errorf("%s is not a directory", dir)
		}
		return "", fmt.Errorf("%s has no %s, so nothing deploys it; add one beside its main.go, or let deploy write one: --to %s",
			dir, cli.EitherOr(configFiles()), cli.EitherOr(cli.SortedKeys(clouds)))
	}
	return "", fmt.Errorf("%s has %s; a directory deploys to one cloud, so split it in two",
		dir, cli.English(cli.Map(found, func(name string) string { return clouds[name].ConfigFile })))
}

// configFiles is what a deployable directory may hold, for a message that has
// to list them.
func configFiles() []string {
	return cli.Sorted(cli.Map(cli.SortedKeys(clouds), func(name string) string { return clouds[name].ConfigFile }))
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// Name is what the app in dir is called on its cloud, suffix included.
func Name(dir, env string) (string, error) {
	to, err := cloudFor(dir)
	if err != nil {
		return "", err
	}
	return to.Name(dir, env)
}

// PutSecret gives the deployed app in dir one secret, through its cloud's own
// CLI, the value on stdin and never an argument.
func PutSecret(dir, env, name, value string) error {
	to, err := cloudFor(dir)
	if err != nil {
		return err
	}
	return to.PutSecret(dir, env, name, value)
}

// scaffold writes a cloud's conventional config into a directory that has
// none, and reports the target it just became. The file is named after the
// directory, as every other name on this stack is.
//
// It is written here rather than by an init verb because that is the pattern
// already: dev makes goreleaser's config at the moment a release needs one, so
// a repo that wants its own writes it and nobody else thinks about it. The one
// difference is that this file stays — the presence of it is what names the
// target, so a temporary one would deploy nowhere the next time anyone looked.
func scaffold(out io.Writer, dir, want string) error {
	to, ok := clouds[want]
	if !ok {
		return cli.Usagef("--to: %v", cli.Unknown("cloud", want, cli.SortedKeys(clouds)))
	}
	if to.Scaffold == nil {
		return fmt.Errorf("--to %s: that target writes no config of its own; add %s by hand", want, to.ConfigFile)
	}
	name := filepath.Base(mustAbs(dir))
	path := filepath.Join(dir, to.ConfigFile)
	if err := os.WriteFile(path, []byte(to.Scaffold(dir, name)), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s, so %s deploys to %s; read it and commit it\n", path, dir, want)
	return nil
}

// mustAbs is dir as an absolute path, falling back to dir when the working
// directory cannot be read — a name is better than a failure here.
func mustAbs(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}

// cloudFor is the target a directory deploys to, as the thing that can act on
// it. One lookup, so that adding a cloud is adding an entry to clouds and
// nothing else — which was true of the verbs and was not true of these two.
func cloudFor(dir string) (cloud, error) {
	target, err := Target(dir)
	if err != nil {
		return cloud{}, err
	}
	to, ok := clouds[target]
	if !ok {
		return cloud{}, cli.Unknown("cloud", target, cli.SortedKeys(clouds))
	}
	return to, nil
}
