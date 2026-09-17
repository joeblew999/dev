// Package cli is the verb system every command on the stack shares: one
// shape for a command (command.go's Command), and the helpers its verbs use
// to read flags and the directory they act on (this file). A command declares
// its verbs and its prose; Main runs it and renders its manual from the same
// table, so the manual cannot drift from the binary.
package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Runner is every dev command: the verb it was called as, the arguments after
// it, and where to write. A package that answers to several verbs (build,
// wasm, check, run, workerd are all stage's) switches on verb.
type Runner func(verb string, args []string, stdout, stderr io.Writer) error

// UsageError means the arguments were wrong; main prints the package's usage
// after it.
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

// Usagef makes a UsageError — unless what it is reporting is a request for
// help, which every verb passes here by writing `Usagef("%s: %v", verb, err)`
// around whatever parsing returned.
//
// It has to be caught here rather than at each call site. Asking every verb in
// every repo to remember one line is asking it to be forgotten, and it was:
// the first fix made propagation the caller's job, and a command in another
// repo, written from the scaffold, still answered --help with
// "error: flag: help requested".
func Usagef(format string, a ...any) error {
	for _, arg := range a {
		if err, ok := arg.(error); ok && errors.Is(err, flag.ErrHelp) {
			return ErrHelp
		}
	}
	return &UsageError{fmt.Sprintf(format, a...)}
}

// Flags is a flag set that reports to stderr and never exits.
func Flags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// Bool is a flag.Value for booleans that also takes "" as false, so a task
// may pass `--flag=$var` with the variable unset.
type Bool bool

func (b *Bool) String() string { return fmt.Sprint(bool(*b)) }

// IsBoolFlag lets a bare `--flag` mean true.
func (b *Bool) IsBoolFlag() bool { return true }

// Set accepts the usual spellings of true and false, and "" as false.
func (b *Bool) Set(s string) error {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "0", "false", "no", "off":
		*b = false
	case "1", "true", "yes", "on":
		*b = true
	default:
		return fmt.Errorf("want true or false, got %q", s)
	}
	return nil
}

// ErrHelp is what a verb returns when it was asked for help rather than run.
// The flag package has already printed the flags and what each one means, so
// the command prints the verb's own usage after it and exits 0: asking what a
// verb takes is not an error, and reporting it as one is how the flags'
// descriptions stayed invisible.
var ErrHelp = flag.ErrHelp

// HelpRequested reports whether args ask what a verb takes, rather than ask
// it to run. Everything after a bare "--" belongs to the program being run, so
// its --help is not ours.
//
// A verb that dispatches on a subcommand, or that wants DIR before anything
// else, has to ask this before it enforces either: otherwise the question is
// answered with "the directory comes first", which is true and useless.
func HelpRequested(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// DirAnd parses "DIR [flags and positionals in any order]": the directory a
// command acts on comes first; positional is how many positionals follow, or
// -1 for any number. Flags may come anywhere, because mise appends what the
// developer typed after a task name to the command; everything after a bare
// "--" is positional, verbatim.
func DirAnd(fs *flag.FlagSet, args []string, positional int) (dir string, rest []string, err error) {
	// Before the directory is required, because asking what the verb takes is
	// how someone finds out that it wants one.
	if HelpRequested(args) {
		fs.Usage()
		return "", nil, ErrHelp
	}
	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		return "", nil, Usagef("%s: the directory comes first", fs.Name())
	}
	rest, err = ParseInterleaved(fs, args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return "", nil, ErrHelp
	}
	if err != nil {
		return "", nil, Usagef("%s: %v", fs.Name(), err)
	}
	if positional >= 0 && len(rest) != positional {
		return "", nil, Usagef("%s: wrong arguments", fs.Name())
	}
	return args[0], rest, nil
}

// ParseInterleaved parses flags wherever they appear and returns the
// positionals, with everything after a bare "--" appended verbatim.
func ParseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals, tail []string
	for i, a := range args {
		if a == "--" {
			args, tail = args[:i], args[i+1:]
			break
		}
	}
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) > 0 {
			positionals = append(positionals, args[0])
			args = args[1:]
		}
	}
	return append(positionals, tail...), nil
}

// Confirm asks on out and reads one line from stdin: y or yes means yes.
// Anything else, including no terminal to answer from, means no.
func Confirm(stdin io.Reader, out io.Writer, prompt string) bool {
	fmt.Fprint(out, prompt)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(out)
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
