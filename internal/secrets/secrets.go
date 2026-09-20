// Package secrets stores a deployed app's secrets in fnox and pushes them to
// the app through its cloud's CLI. Names arrive on the command line or stdin;
// values only ever pass through fnox and that CLI. It runs as `dev secrets`.
package secrets

import (
	"bufio"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/app"
	"github.com/joeblew999/dev/internal/fnox"
	"github.com/joeblew999/dev/internal/gitrepo"

	"github.com/joeblew999/dev/cli/tool"
)

// Usage is what dev prints for these verbs. It is markdown in a file beside
// this one, not a string const: a Go raw string is backtick-delimited, so it
// can never hold the inline code that keeps a `<placeholder>` from reaching a
// markdown renderer as an HTML tag.

//go:embed usage.md
var Usage string

// Run is `dev secrets set|push`.
// Subs are secrets' subcommands, each declaring the flags it registers.
// main.go hands these to cli, which renders every signature from them, and Run
// registers the same ones — so `dev secrets push --help` and the manual show
// the same flags because they are the same registration.
var Subs = map[string]cli.Verb{
	"set":  {Run: runSet, Args: "DIR NAME|OWNER", Flags: SetFlags, Desc: "store one secret and push it to the app, in a single step"},
	"push": {Run: runPush, Args: "DIR", Flags: PushFlags, Desc: "push every secret an app needs, read as a list on stdin"},
	"ci":   {Run: runCI, Args: "NAME...", Desc: "give GitHub Actions the secrets it needs to sign and deploy"},
}

// EnvFlag is the environment every secrets subcommand pushes to.
func EnvFlag(fs *flag.FlagSet) {
	fs.String("env", "", "wrangler environment `NAME` to push to")
}

// SetFlags are what `secrets set` takes.
func SetFlags(fs *flag.FlagSet) {
	EnvFlag(fs)
	fs.Var(new(cli.Bool), "generate", "make a random 64-hex-character value instead of prompting")
	fs.Var(new(cli.Bool), "if-missing", "do nothing when fnox already has the secret")
	fs.String("names", "", "the project's `LIST` of NAME<TAB>OWNER lines, so an owner resolves to its secret")
}

// PushFlags are what `secrets push` takes.
func PushFlags(fs *flag.FlagSet) {
	EnvFlag(fs)
	fs.String("fix", "mise run secrets:set {provider}", "the `TEMPLATE` to run for a secret fnox does not have")
}

// set, push and ci are the three subcommands, each the work it does. cli
// parses what each declared in Subs, so none of them builds a FlagSet or
// pulls a directory out of the arguments for itself.
func runSet(c cli.Call) error {
	name, err := Resolve(c.Value("names"), c.Args[0])
	if err != nil {
		return err
	}
	return Set(c.Stdin, c.Stdout, c.Stderr, name, c.Given("generate"), c.Given("if-missing"), c.Dir, c.Value("env"))
}

func runPush(c cli.Call) error {
	return Push(c.Stdin, c.Stdout, c.Dir, c.Value("env"), c.Value("fix"))
}

func runCI(c cli.Call) error {
	for _, name := range c.Args {
		v, err := fnox.Get(name)
		if err != nil || v == "" {
			return fmt.Errorf("%s is not in fnox; store it with: fnox set -g %s", name, name)
		}
		if err := CI(name, v); err != nil {
			return err
		}
		fmt.Fprintf(c.Stdout, "set %s in this repo's Actions secrets\n", name)
	}
	return nil
}

// Resolve turns what the developer typed into a secret name, given the
// project's "NAME<TAB>OWNER" lines: an owner (a provider, "admin") maps to its
// name, a name maps to itself, anything else is an error naming what exists.
func Resolve(names, arg string) (string, error) {
	var known []string
	for _, line := range cli.Lines(names) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		known = append(known, fields[0])
		if fields[0] == arg || (len(fields) > 1 && fields[1] == arg) {
			return fields[0], nil
		}
	}
	if strings.TrimSpace(names) == "" {
		return arg, nil
	}
	return "", fmt.Errorf("%q is not a secret or an owner this project knows; the secrets are: %s", arg, strings.Join(known, ", "))
}

// Set stores one secret in fnox and pushes it to the app. The value is
// generated, or read hidden from a terminal, or read as one line from a pipe.
func Set(stdin io.Reader, stdout, stderr io.Writer, name string, generate, ifMissing bool, dir, env string) error {
	if ifMissing {
		if v, err := fnox.Get(name); err == nil && v != "" {
			fmt.Fprintf(stdout, "%s is already in fnox; leaving it\n", name)
			return nil
		}
	}
	value, err := value(stdin, stderr, name, generate)
	if err != nil {
		return err
	}
	if err := fnox.Set(name, value); err != nil {
		return fmt.Errorf("storing %s in fnox: %w", name, err)
	}
	fmt.Fprintf(stdout, "Stored %s in fnox.\n", name)
	if err := app.PutSecret(dir, env, name, value); err != nil {
		return fmt.Errorf("pushing %s to the app: %w (retry with: mise run secrets:push)", name, err)
	}
	target, _ := app.Name(dir, env)
	fmt.Fprintf(stdout, "Pushed %s to %s.\n", name, target)
	return nil
}

func value(stdin io.Reader, prompt io.Writer, name string, generate bool) (string, error) {
	if generate {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		return hex.EncodeToString(b), nil
	}
	var v string
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprintf(prompt, "Value for %s (input hidden): ", name)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(prompt)
		if err != nil {
			return "", err
		}
		v = string(b)
	} else {
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		v = strings.TrimRight(line, "\r\n")
	}
	if v == "" {
		return "", fmt.Errorf("no value given for %s; nothing was changed", name)
	}
	return v, nil
}

// Push reads "NAME<TAB>OWNER" lines (owner optional) and pushes every named
// secret from fnox to the app. A secret fnox does not have is reported
// with fix, {provider} replaced, and the error at the end carries the count
// so the caller exits non-zero.
func Push(stdin io.Reader, out io.Writer, dir, env, fix string) error {
	problems := 0
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		name, owner := fields[0], fields[0]
		if len(fields) > 1 {
			owner = fields[1]
		}
		v, err := fnox.Get(name)
		if err != nil || v == "" {
			fmt.Fprintf(out, "missing %s -> %s\n", name, strings.ReplaceAll(fix, "{provider}", owner))
			problems++
			continue
		}
		if err := app.PutSecret(dir, env, name, v); err != nil {
			fmt.Fprintf(out, "failed  %s (%v; retry with: mise run secrets:push)\n", name, err)
			problems++
			continue
		}
		fmt.Fprintf(out, "pushed  %s\n", name)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if problems > 0 {
		return fmt.Errorf("%d secret(s) not pushed", problems)
	}
	return nil
}

// GhBin is the CLI that sets the repo's Actions secrets.
const GhBin = "gh"

// CI sets one of the repo's GitHub Actions secrets, the value on stdin. A
// variable so tests can replace it.
var CI = func(name, value string) error {
	slug, err := gitrepo.Slug(".")
	if err != nil {
		return err
	}
	err = tool.Cmd{Bin: GhBin, Pin: `gh = "latest"`, Args: []string{"secret", "set", name, "--repo", slug}, Stdin: strings.NewReader(value)}.Stream(io.Discard)
	if err != nil {
		return fmt.Errorf("gh secret set %s failed: %w (gh must be logged in with access to this repo)", name, err)
	}
	return nil
}
