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

// Exec runs a command in dir with fnox's secrets in its environment.
var Exec = func(dir string, stdin io.Reader, stdout io.Writer, args ...string) error {
	return tool.Cmd{Bin: Bin, Pin: Pin, Args: append([]string{"exec", "--"}, args...), Dir: dir, Stdin: stdin}.Stream(stdout)
}

// Ask runs a command with fnox's secrets and returns everything it said,
// stdout and stderr together.
//
// Exec streams stdout and lets stderr reach the terminal, which is right for a
// command whose output is for the person watching. It is wrong for a question:
// a CLI answering "is this there" almost always answers on stderr, and reading
// only stdout means never seeing the answer. That is exactly how `flyctl
// status` saying `Could not find App` was missed, turning "no such app" into
// an unrecognised failure.
//
// Quiet, because a question asked on the way to doing something should not
// report its own timing as though it were the work.
var Ask = func(dir string, args ...string) (string, error) {
	res, err := tool.Cmd{Bin: Bin, Pin: Pin, Dir: dir, Quiet: true, Combined: true,
		Args: append([]string{"exec", "--"}, args...)}.Capture()
	if err != nil {
		return res.Out, err
	}
	// Capture keeps no error for a program that failed but printed something,
	// because for a checker the non-zero exit is the answer. For a question it
	// is the opposite: the whole point is whether the program succeeded, and
	// reading that nil as yes is how `flyctl status` saying "Could not find
	// App" was taken for "the app is there".
	if !res.OK() {
		return res.Out, fmt.Errorf("%s exited %d", strings.Join(args, " "), res.Code)
	}
	return res.Out, nil
}
