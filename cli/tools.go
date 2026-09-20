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
	"flag"
	"fmt"
	"os/exec"
)

// ToolsFlags are what tools takes.
func ToolsFlags(fs *flag.FlagSet) {
	fs.Var(new(Bool), "missing", "only what this machine does not have")
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
	var found []entry
	for _, n := range Needs() {
		_, err := exec.LookPath(n.Bin)
		e := entry{Bin: n.Bin, Pin: n.Pin, For: n.For, Present: err == nil, Managed: n.Pin != ""}
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
	// The lines to paste, for the half mise can install. A repo pins what it
	// will actually run, so this is not written into anybody's file: a repo
	// that never deploys to Fly should not carry flyctl to satisfy a checker.
	var lines []string
	for _, e := range found {
		if e.Managed && !e.Present {
			lines = append(lines, "  "+e.Pin)
		}
	}
	if len(lines) > 0 {
		fmt.Fprintf(call.Stdout, "\nfor mise.toml [tools], the ones you will run:\n")
		for _, l := range lines {
			fmt.Fprintln(call.Stdout, l)
		}
	}
	return nil
}
