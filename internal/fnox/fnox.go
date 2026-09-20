// Package fnox is the one way dev reaches a secret. fnox holds every
// credential, and wrangler only ever runs under `fnox exec`, which is how it
// gets CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID without either touching
// a file in the repo. The functions are variables so tests can replace them.
package fnox

import (
	"fmt"
	"io"
	"strings"

	"github.com/joeblew999/dev/cli/tool"
)

// Bin is the fnox CLI every function here shells out to.
const Bin = "fnox"

// Pin is what installs it, quoted when it is missing.
const Pin = `fnox = "latest"`

// Get returns a secret's value, or an error when fnox does not have it.
var Get = func(name string) (string, error) {
	res, err := tool.Cmd{Bin: Bin, Pin: Pin, Args: []string{"get", name}, Quiet: true}.Capture()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Out), nil
}

// Set stores a value in the developer's global fnox config. The value goes in
// on stdin, so it is never an argument, a process list or a shell history.
var Set = func(name, value string) error {
	return tool.Cmd{Bin: Bin, Pin: Pin, Args: []string{"set", "-g", name}, Stdin: strings.NewReader(value)}.Stream(io.Discard)
}

// Under is one command to run with fnox's secrets in its environment: where,
// what, what it reads, and where its output goes.
type Under struct {
	Dir   string    // where to run it; "" is the current directory
	Args  []string  // the command and its arguments, after `fnox exec --`
	Stdin io.Reader // what it reads, for a value handed over rather than argued
	Out   io.Writer // where its stdout goes; nil keeps it

	// Combined captures stderr as well and hands both back. A command whose
	// output is for the person watching wants it off; a question wants it on,
	// because a CLI answering "is this there" answers on stderr.
	Combined bool
}

// Run is the one way into fnox, and the one thing a test replaces.
//
// There were two — Exec for work and Ask for a question — and three separate
// tests stubbed Exec, then kept passing while running the real fnox against
// the real account the moment their code moved a call to Ask. Two doors means
// a test can only ever guard one of them, and it cannot tell that it missed.
// Exec and Ask are ordinary functions over this now, so stubbing Run catches
// everything, including whatever is written next.
var Run = func(u Under) (string, error) {
	cmd := tool.Cmd{Bin: Bin, Pin: Pin, Dir: u.Dir, Stdin: u.Stdin,
		Args: append([]string{"exec", "--"}, u.Args...)}
	if !u.Combined {
		if u.Out == nil {
			u.Out = io.Discard
		}
		return "", cmd.Stream(u.Out)
	}
	cmd.Quiet, cmd.Combined = true, true
	res, err := cmd.Capture()
	if err != nil {
		return res.Out, err
	}
	// Capture keeps no error for a program that failed but printed something,
	// because for a checker the non-zero exit is the answer. For a question it
	// is the opposite: the whole point is whether the program succeeded, and
	// reading that nil as yes is how `flyctl status` saying "Could not find
	// App" was taken for "the app is there".
	if !res.OK() {
		return res.Out, fmt.Errorf("%s exited %d", strings.Join(u.Args, " "), res.Code)
	}
	return res.Out, nil
}

// Exec runs a command in dir with fnox's secrets, its stdout going to stdout
// and its own progress to the terminal. For work being watched.
func Exec(dir string, stdin io.Reader, stdout io.Writer, args ...string) error {
	_, err := Run(Under{Dir: dir, Args: args, Stdin: stdin, Out: stdout})
	return err
}

// Ask runs a command with fnox's secrets and returns everything it said,
// stdout and stderr together, reporting a non-zero exit as the error it is.
// For a question rather than for work.
func Ask(dir string, args ...string) (string, error) {
	return Run(Under{Dir: dir, Args: args, Combined: true})
}
