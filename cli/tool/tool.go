// Package tool runs another program: the one way anything on this stack
// reaches a binary it does not contain.
//
// A package of its own, beside cli rather than in it, for the reason
// skillcheck is: cli is linked into every command built on it, a Worker's
// wasm included, and a Worker can never exec anything. What only a command on
// a developer's machine needs stays out of cli.
//
// What it adds over exec.Command is what every caller wanted and each wrote
// differently: a missing binary reported as the line to add to mise.toml
// rather than as "executable file not found in $PATH", stdout captured while
// the program's own progress still reaches the terminal, and every run timed,
// so a command that runs six checkers can say which one was slow.
package tool

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/joeblew999/dev/cli"
)

// Cmd is one program to run, and how. The zero value runs Bin with no
// arguments in the current directory; every other field is one a caller sets
// only when it needs it, so nobody passes six arguments it does not use.
type Cmd struct {
	Bin  string   // the program
	Pin  string   // the mise.toml [tools] line that installs it, quoted when it is missing
	Args []string // its arguments
	Dir  string   // where to run it; "" is the current directory

	Env      []string      // added to the environment, as NAME=VALUE
	Stdin    io.Reader     // what it reads, for a program handed a value rather than an argument
	Combined bool          // capture what it writes to stderr as well
	Timeout  time.Duration // give up after this long; 0 waits
	Quiet    bool          // do not say how long it took, however long that was
}

// Result is one run: what it printed, and how long it took. Timing is why
// this is a struct rather than a string — "which tool is slow" has to be
// answered by the run itself, not by a person with a stopwatch.
type Result struct {
	Bin  string        `json:"bin"`
	Out  string        `json:"-"`
	Took time.Duration `json:"took"`

	// Code is the program's exit status, and -1 when it never ran. Capture
	// returns no error for a program that failed but printed something —
	// which is right for a checker, whose non-zero exit is its answer, and
	// wrong for every caller asking a question. Twice in one session a caller
	// read that nil as success: a wasm check, and `flyctl status`, which
	// answers "Could not find App" and exits 1 and was taken for "the app is
	// there". The status was discarded, so each caller had to re-derive it by
	// matching the output text. It is here now.
	Code int `json:"code"`
}

// OK reports whether the program exited zero, which is the question a caller
// asking a question wants answered.
func (r Result) OK() bool { return r.Code == 0 }

// JSON is what this run printed, decoded into T.
//
// A method with its own type parameter, which Go 1.27 allows: the receiver
// already holds the output and which tool produced it, so the only thing left
// to say is the shape expected back — res.JSON[scoutlyReport]("scoutly").
func (r Result) JSON[T any](what string) (T, error) {
	return cli.DecodeJSON[T](what, r.Out)
}

// Capture runs the program and returns what it printed on stdout. Stderr
// reaches the terminal, because a program's progress is for the person
// watching and not for the caller's data — unless Combined asks for both.
func (c Cmd) Capture() (Result, error) {
	cmd, cancel, err := c.build()
	if err != nil {
		return Result{Bin: c.Bin}, err
	}
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, os.Stderr
	if c.Combined {
		cmd.Stderr = &out
	}
	started := time.Now()
	runErr := cmd.Run()
	res := Result{Bin: c.Bin, Out: out.String(), Took: time.Since(started), Code: exitCode(cmd, runErr)}
	report(res, c.Quiet)
	// A checker exits non-zero because it found something, which is its
	// answer and not a failure to run: hand back what it printed and let the
	// caller decide. Nothing printed and an error means it really failed.
	if runErr != nil && out.Len() == 0 {
		return res, fmt.Errorf("%s %v: %w", c.Bin, c.Args, runErr)
	}
	return res, nil
}

// Attached runs the program wired to this terminal: the caller sees it
// exactly as if they had typed it, and it runs until it exits or is
// interrupted. For a log stream, an installer, a release — anything whose
// output is for the person rather than for the caller.
func (c Cmd) Attached() error {
	cmd, cancel, err := c.build()
	if err != nil {
		return err
	}
	defer cancel()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if c.Stdin != nil {
		cmd.Stdin = c.Stdin
	}
	started := time.Now()
	runErr := cmd.Run()
	report(Result{Bin: c.Bin, Took: time.Since(started)}, c.Quiet)
	return runErr
}

// Stream runs the program with its stdout going where the caller says and
// its stderr to the terminal. That is the shape of every build, deploy and
// secret push here: the person watches the tool's own progress, and whatever
// it produces goes where the caller wanted it.
func (c Cmd) Stream(out io.Writer) error {
	cmd, cancel, err := c.build()
	if err != nil {
		return err
	}
	defer cancel()
	cmd.Stdout, cmd.Stderr = out, os.Stderr
	started := time.Now()
	runErr := cmd.Run()
	report(Result{Bin: c.Bin, Took: time.Since(started)}, c.Quiet)
	if runErr != nil {
		return fmt.Errorf("%s %v (in %s): %w", c.Bin, c.Args, cli.Or(c.Dir, "."), runErr)
	}
	return nil
}

// Start runs the program in the background and hands it back, for the two
// things that need one alive while something else happens: a server to make a
// request against, and a browser to drive. The caller stops it.
func (c Cmd) Start(stdout, stderr io.Writer) (*exec.Cmd, error) {
	return c.Started(stdout, stderr)
}

// Started is Start with something to do to the command first, for the one
// caller that needs its own process group so stopping it stops what it
// spawned. Variadic rather than another field, because it is a func and a
// zero value of one says nothing.
func (c Cmd) Started(stdout, stderr io.Writer, before ...func(*exec.Cmd)) (*exec.Cmd, error) {
	cmd, _, err := c.build()
	if err != nil {
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	for _, f := range before {
		f(cmd)
	}
	return cmd, cmd.Start()
}

// build is the program, looked up and wired. The lookup is the point: a tool
// this repo pins is on PATH inside the repo and nowhere else, so "not found"
// is nearly always "you are outside the repo, or nobody added the pin", and
// neither is something a person should have to work out from exec's wording.
func (c Cmd) build() (*exec.Cmd, context.CancelFunc, error) {
	nothing := context.CancelFunc(func() {})
	if _, err := exec.LookPath(c.Bin); err != nil {
		// The caller's own wording wins, then what the tool registry knows.
		// Every caller used to have to remember a pin, and thirteen of the
		// fourteen did not, so a missing binary said only that it was
		// missing — which is useless to the one person it matters to, the
		// one who has just adopted dev and does not know what it wants.
		switch pin := cmp.Or(c.Pin, cli.PinFor(c.Bin)); {
		case pin == "":
			return nil, nothing, fmt.Errorf("%s is not on PATH", c.Bin)
		case !cli.Installable(c.Bin) && c.Pin == "":
			return nil, nothing, fmt.Errorf("%s is not on PATH: %s", c.Bin, pin)
		default:
			return nil, nothing, fmt.Errorf("%s is not on PATH; add it to mise.toml [tools] and run mise install:\n  %s", c.Bin, pin)
		}
	}
	ctx, cancel := context.Background(), nothing
	if c.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
	}
	cmd := exec.CommandContext(ctx, c.Bin, c.Args...)
	cmd.Dir, cmd.Stdin = c.Dir, c.Stdin
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	return cmd, cancel, nil
}

// Stop ends a program started with Start and waits for it to go. Whether
// that is a signal to a process group or a kill is the platform's business
// and lives beside it; "is there anything to stop, and wait for it" is not,
// and was written out on both sides.
func Stop(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	end(cmd)
	_, _ = cmd.Process.Wait()
}

// Timing is where every run says how long it took: stderr, like every other
// progress line, because stdout belongs to whatever the caller is building. A
// variable so a test can silence it, and one place so no caller has to
// remember to say it.
var Timing io.Writer = os.Stderr

// Worth is how long a run has to take before saying so is worth a line. Every
// run is measured and every Result carries its time; this is only about what
// reaches the terminal, where one line per fnox read would bury the tool that
// actually took nine seconds.
var Worth = 100 * time.Millisecond

func report(r Result, quiet bool) {
	if Timing != nil && !quiet && r.Took >= Worth {
		fmt.Fprintf(Timing, "  %s: %s\n", r.Bin, cli.Took(r.Took))
	}
}

// Run is Capture for the common case: a tool, its pin, its arguments. A
// variable, so a test can hand back a recorded run instead.
var Run = func(bin, pin string, args ...string) (Result, error) {
	return Cmd{Bin: bin, Pin: pin, Args: args}.Capture()
}

// Attached runs bin wired to the terminal, in dir.
func Attached(dir, bin string, args ...string) error {
	return Cmd{Bin: bin, Dir: dir, Args: args}.Attached()
}

// exitCode is what the program exited with: 0 when it succeeded, its status
// when it failed, and -1 when it never ran at all — a missing binary, a
// directory that is not there — which is not an exit status and must not be
// mistaken for one.
func exitCode(cmd *exec.Cmd, runErr error) int {
	if runErr == nil {
		return 0
	}
	if exit, ok := errors.AsType[*exec.ExitError](runErr); ok {
		return exit.ExitCode()
	}
	return -1
}
