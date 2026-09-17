package cli

import (
	"flag"
	"io"
	"strings"
	"testing"
)

func TestDirAndTakesFlagsAnywhere(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	a := fs.Bool("a", false, "")
	b := fs.String("b", "", "")
	dir, rest, err := DirAnd(fs, []string{"cmd/x", "-b", "one", "first", "-a"}, 1)
	if err != nil || dir != "cmd/x" || !*a || *b != "one" || strings.Join(rest, ",") != "first" {
		t.Fatalf("got %q %v %v a=%v b=%q", dir, rest, err, *a, *b)
	}
	if _, _, err := DirAnd(fs, []string{"-a", "cmd/x"}, 0); err == nil {
		t.Fatal("a flag before the directory was accepted")
	}
}

func TestParseInterleavedPassesEverythingAfterDoubleDash(t *testing.T) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	env := fs.String("env", "", "")
	got, err := ParseInterleaved(fs, []string{"--env", "x", "--", "serve", "--config", "f.toml"})
	if err != nil || *env != "x" || strings.Join(got, " ") != "serve --config f.toml" {
		t.Fatalf("got %q %v env=%q", got, err, *env)
	}
}

// A verb whose Args begin with DIR gets its directory parsed by cli, so no
// verb does it for itself. mise appends what a developer typed after a task
// name, so flags may follow the directory but never precede it — and saying
// so is cli's job, since cli is what reads them.
func TestParseTakesTheDirectoryFirst(t *testing.T) {
	v := Verb{Args: "DIR", Flags: func(fs *flag.FlagSet) { fs.String("env", "", "an `ENV`") }}

	c, err := v.parse("deploy", []string{"cmd/x", "--env", "prod", "extra"}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Dir != "cmd/x" {
		t.Errorf("Dir = %q, want cmd/x", c.Dir)
	}
	if c.Value("env") != "prod" {
		t.Errorf("--env = %q, want prod", c.Value("env"))
	}
	if len(c.Args) != 1 || c.Args[0] != "extra" {
		t.Errorf("Args = %v, want [extra]", c.Args)
	}

	if _, err := v.parse("deploy", []string{"--env", "prod"}, io.Discard, io.Discard); err == nil ||
		!strings.Contains(err.Error(), "the directory comes first") {
		t.Errorf("flags before DIR should say so, got %v", err)
	}
}
