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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ToolsFlags are what tools takes.
func ToolsFlags(fs *flag.FlagSet) {
	fs.Var(new(Bool), "missing", "only what this machine does not have")
	fs.Var(new(Bool), "add", "write the missing [tools] lines into this repo's mise.toml")
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
	// The lines to paste, for the half mise can install. A repo pins what it
	// will actually run, so this is not written into anybody's file: a repo
	// that never deploys to Fly should not carry flyctl to satisfy a checker.
	var lines []string
	for _, e := range found {
		if e.Managed && !e.Present {
			lines = append(lines, "  "+e.Pin)
		}
	}
	if len(lines) == 0 {
		return nil
	}
	if !call.Given("add") {
		fmt.Fprintf(call.Stdout, "\nfor mise.toml [tools], the ones you will run:\n")
		for _, l := range lines {
			fmt.Fprintln(call.Stdout, l)
		}
		fmt.Fprintf(call.Stdout, "\nOr let this write them: %s tools --add\n", c.Name)
		return nil
	}
	return addTools(call, lines)
}

// addTools writes the missing pins into the repo's mise.toml.
//
// Appended to the [tools] table rather than rewritten, and only lines that
// are not there: a repo's mise.toml is its own, holding versions somebody
// chose and comments explaining why, and a tool that reformats it to add a
// line will not be run twice.
func addTools(call Call, lines []string) error {
	dir, err := root(".")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "mise.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w; this is a repo on the stack, so it has one", rel(path), err)
	}
	text := string(data)
	var added []string
	for _, l := range lines {
		pin := strings.TrimSpace(l)
		name, _, _ := strings.Cut(pin, " =")
		// Already pinned, whatever version it names: a repo that chose 1.24
		// should not be given "latest" underneath it.
		if strings.Contains(text, "\n"+name+" =") || strings.Contains(text, "\n"+name+"=") {
			continue
		}
		added = append(added, pin)
	}
	if len(added) == 0 {
		fmt.Fprintln(call.Stdout, "\nmise.toml already pins everything that is missing")
		return nil
	}
	marker := "[tools]"
	at := strings.Index(text, marker)
	if at < 0 {
		return fmt.Errorf("%s has no [tools] table to add to", rel(path))
	}
	insert := at + len(marker)
	written := text[:insert] + "\n" + strings.Join(added, "\n") + text[insert:]
	if err := os.WriteFile(path, []byte(written), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(call.Stdout, "\nadded to %s:\n", rel(path))
	for _, l := range added {
		fmt.Fprintf(call.Stdout, "  %s\n", l)
	}
	fmt.Fprintln(call.Stdout, "\nthen: mise install")
	return nil
}

// here reports whether a tool can actually be run in this directory.
//
// Not the same as being on PATH, which was the first answer and the wrong
// one: mise puts a shim on PATH for every tool it knows about, so LookPath
// finds flyctl in a repo that pins no flyctl and the shim fails only when
// run. Everything looked installed and nothing was.
//
// So mise is asked what is active here, and PATH is only consulted for the
// three it does not manage.
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
			// A tool is listed by its [tools] key, which may be a backend
			// path — "npm:wrangler", "packslip:github.com/jdx/fnox" — and
			// the binary is the last word of it.
			bin := name
			if i := strings.LastIndexAny(bin, ":/"); i >= 0 {
				bin = bin[i+1:]
			}
			active[bin] = true
		}
	}
	return active
}
