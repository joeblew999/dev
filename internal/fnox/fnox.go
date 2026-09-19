// Package fnox is the one way dev reaches a secret. fnox holds every
// credential, and wrangler only ever runs under `fnox exec`, which is how it
// gets CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID without either touching
// a file in the repo. The functions are variables so tests can replace them.
package fnox

import (
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
