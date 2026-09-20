// What a repo has to pin before it can use this command.
//
// A repo adopts dev by pinning one line in mise.toml, and then finds out
// what else it needs one failure at a time: wrangler when it first deploys a
// Worker, node because wrangler runs on it, flyctl when it first deploys to
// Fly, fnox the moment anything touches a secret. Each of those is a stop,
// a search, and a guess at a version.
//
// `<cmd> tools` is the list up front, and which of them this machine already
// has. It reads the same declaration the missing-binary error reads, so the
// list and the error cannot disagree.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ToolsFlags are what tools takes.
func ToolsFlags(fs *flag.FlagSet) {
	fs.Var(new(Bool), "missing", "only what this machine does not have")
	fs.Var(new(Bool), "add", "write the missing [tools] lines into this repo's mise.toml")
	fs.Var(new(Bool), "fresh", "replace what is pinned with exactly what this command needs, making a config when there is none")
	fs.Var(new(Bool), "yes", "do not ask before replacing an existing config")
	JSONFlags(fs)
}

// tools is `<cmd> tools`: every program this command may run, the mise line
// that installs it, and whether it is here.
func (c Command) tools(call Call) error {
	type entry struct {
		Bin     string `json:"bin"`
		Pin     string `json:"pin,omitempty"`
		For     string `json:"for"`
		Present bool   `json:"present"`
		// Managed says whether mise can install it. The three it cannot —
		// git, and the operating system's own ps and lsof — are listed all
		// the same, because "dev needs this" is true of them too and a repo
		// building an image has to know.
		Managed bool `json:"managed"`
	}
	active := miseActive()
	var found []entry
	for _, n := range Needs() {
		e := entry{Bin: n.Bin, Pin: n.Pin, For: n.For, Present: here(n, active), Managed: n.Pin != ""}
		if call.Given("missing") && e.Present {
			continue
		}
		found = append(found, e)
	}
	if call.WantsJSON() {
		return call.EmitJSON(found)
	}
	for _, e := range found {
		mark := "missing"
		if e.Present {
			mark = "ok"
		}
		fmt.Fprintf(call.Stdout, "%-7s %-12s %s\n", mark, e.Bin, e.For)
	}
	// What mise could install and this directory does not have. A repo pins
	// what it will actually run, so nothing is written unless asked: one that
	// never deploys to Fly should not carry flyctl to satisfy a checker.
	var want []Need
	for _, n := range Needs() {
		if n.Pin != "" && !here(n, active) {
			want = append(want, n)
		}
	}
	if call.Given("fresh") {
		return c.freshTools(call, all(Needs()))
	}
	if len(want) == 0 {
		return nil
	}
	if !call.Given("add") {
		fmt.Fprintf(call.Stdout, "\nfor mise.toml [tools], the ones you will run:\n")
		for _, n := range want {
			fmt.Fprintf(call.Stdout, "  %s\n", n.Line())
		}
		fmt.Fprintf(call.Stdout, "\nOr have mise record them: %s tools --add\n", c.Name)
		return nil
	}
	return addTools(call, want)
}

// all is every tool mise can install, which is what fresh establishes: the
// question there is not what is missing but what a working one looks like.
func all(needs []Need) []Need {
	return Filter(needs, func(n Need) bool { return n.Pin != "" })
}

// addTools has mise record the missing pins, leaving what is there alone.
func addTools(call Call, needs []Need) error {
	return record(call, needs, false)
}

// freshTools replaces what is pinned with exactly what this command needs.
//
// The other half of a pattern this stack keeps arriving at: something that
// reconciles toward a state, and something that establishes one. sync and
// remove, front and unfront, add and fresh. Add is right for a repo somebody
// owns; fresh is right for a scratch directory, where the question is not
// "what is missing" but "give me one that works".
func (c Command) freshTools(call Call, needs []Need) error {
	return record(call, needs, true)
}

// record is both of them: the same work, differing only in whether what is
// already pinned survives.
//
// Through `mise use` rather than by editing a config, because mise owns that
// file. It knows which of several spellings this directory uses, it creates
// one when there is none, it spells a backend correctly, and it installs what
// it records. Writing the TOML by hand put a duplicate key in a [tools] table
// the first time it was tried, which is the argument in one line.
//
// mise itself is never installed, and never written outside this directory.
func record(call Call, needs []Need, fresh bool) error {
	if err := haveMise(); err != nil {
		return err
	}
	existing := configHere()
	switch {
	case !fresh && existing != "":
		fmt.Fprintf(call.Stdout, "adding to %s\n", existing)
	case !fresh:
		fmt.Fprintf(call.Stdout, "no mise config in this directory; mise will make one\n")
	case existing == "":
		fmt.Fprintf(call.Stdout, "no mise config in this directory; making one with %s\n", Plural(len(needs), "tool"))
	default:
		fmt.Fprintf(call.Stdout, "%s already exists here.\nIts tools will be replaced with %s.\n\n", existing, Plural(len(needs), "tool"))
		if !call.Given("yes") && !Confirm(call.Stdin, call.Stdout, "replace? [y/N] ") {
			return errors.New("not replaced (pass --yes to skip the question)")
		}
		// Emptied rather than deleted, and only ever this directory's own
		// file: what is replaced is the set of tools, and a config may hold
		// tasks and settings that are nobody's business here.
		if err := os.WriteFile(existing, []byte("[tools]\n"), 0o644); err != nil {
			return err
		}
	}
	specs := Unique(Collect(needs, func(n Need) (string, bool) { return n.Spec(), n.Pin != "" }))
	// --path names this directory's own file, so mise cannot write anywhere
	// else. Without it, mise picks by its own precedence, and in a directory
	// with no config of its own that walks up to the machine's global one —
	// which is never this command's to touch.
	at := existing
	if at == "" {
		at = "mise.toml"
	}
	fmt.Fprintf(call.Stdout, "\nmise use --path %s %s\n\n", at, strings.Join(specs, " "))
	return mise(call, append([]string{"use", "--path", at}, specs...))
}

// haveMise refuses rather than installs.
//
// mise is what installs things, and a command about deploying should not put
// a version manager on somebody's machine as a side effect. Saying where to
// get it is the whole of what is appropriate here.
func haveMise() error {
	if _, err := exec.LookPath("mise"); err != nil {
		return errors.New("mise is not installed, and this will not install it: it is a tool that manages your machine's tools, so that is your call. https://mise.jdx.dev/getting-started.html")
	}
	return nil
}

// configHere is the mise config in this directory, or "" when there is none.
//
// This directory and nowhere else. Asking mise which config it would write to
// answers with the whole precedence chain, and the first line of that is the
// global one — so a --fresh in an empty scratch directory read
// ~/.config/mise/config.toml as "the config here" and set about replacing a
// machine's own settings. It failed on a path quirk rather than on judgement,
// which is the kind of luck not to rely on twice.
func configHere() string {
	for _, name := range []string{"mise.toml", ".mise.toml", "mise/config.toml"} {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}
	return ""
}

// miseBinary is what a [tools] key puts on PATH. A key may be a backend path
// — "npm:wrangler", "packslip:github.com/jdx/fnox", "go:github.com/mibk/dupl"
// — and the binary is the last word of it.
//
// A Go module path may end in its major version, and that is never the
// binary: "go:github.com/raviqqe/muffet/v2" installs muffet, and reading the
// last word alone made it "v2" — so muffet was reported missing in a repo
// that pins it and has it, which is the same wrong answer Need.Key exists to
// prevent, arriving by a different road.
//
// One function because two callers have to agree: mise's listing is read
// through it, and the test that holds a Need's key to its pin has to strip
// the pin the same way or it fails a pin that works.
func miseBinary(key string) string {
	for range 2 {
		i := strings.LastIndexAny(key, ":/")
		if i < 0 {
			break
		}
		last := key[i+1:]
		if !majorVersion(last) {
			return last
		}
		key = key[:i]
	}
	return key
}

// majorVersion reports whether a path segment is a Go module's major version
// — "v2", "v11" — rather than the name of what it builds.
func majorVersion(segment string) bool {
	rest, ok := strings.CutPrefix(segment, "v")
	if !ok || rest == "" {
		return false
	}
	return strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' }) < 0
}

// mise runs it where the person can see what it did.
func mise(call Call, args []string) error {
	cmd := exec.Command("mise", args...)
	cmd.Stdout, cmd.Stderr = call.Stdout, call.Stderr
	return cmd.Run()
}

// here reports whether a tool can actually be run in this directory.
//
// Not the same as being on PATH, which was the first answer and the wrong
// one: mise puts a shim on PATH for every tool it knows about, so LookPath
// finds flyctl in a repo that pins no flyctl and the shim fails only when
// run. Everything looked installed and nothing was.
//
// So mise is asked what is active here, and PATH is only consulted for the
// few it does not manage.
func here(n Need, active map[string]bool) bool {
	if n.Pin == "" {
		_, err := exec.LookPath(n.Bin)
		return err == nil
	}
	return active[n.MiseKey()]
}

// miseActive is what mise says this directory has, by the name a [tools] line
// gives it. An empty answer means mise could not be asked, and then nothing
// is claimed to be present rather than everything.
func miseActive() map[string]bool {
	out, err := exec.Command("mise", "ls", "--current", "--json").Output()
	if err != nil {
		return map[string]bool{}
	}
	var listed map[string][]struct {
		Version   string `json:"version"`
		Installed bool   `json:"installed"`
	}
	if err := json.Unmarshal(out, &listed); err != nil {
		return map[string]bool{}
	}
	active := map[string]bool{}
	for name, versions := range listed {
		for _, v := range versions {
			if !v.Installed {
				continue
			}
			active[miseBinary(name)] = true
		}
	}
	return active
}
