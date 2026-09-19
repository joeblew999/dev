// What this verb can do, read out of the registries rather than written
// beside them: a new writer or checker appears here the moment it exists,
// and cannot be added without saying what it needs, what it gives and what
// it costs.
//
// It runs nothing and touches no network, so an agent can read it before
// starting work and know what to ask the operator for.
package seo

import (
	"fmt"
	"os/exec"

	"github.com/joeblew999/dev/cli"
)

// Can is one thing this verb can do.
type Can struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // write, or check
	Provides  string `json:"provides"`
	Cost      string `json:"typicalCost,omitempty"`
	Needs     string `json:"needs,omitempty"`    // the mise.toml line that installs it
	Produces  string `json:"produces,omitempty"` // the file a writer makes
	Installed bool   `json:"installed"`
}

// Capabilities is every writer and every checker, in the order they run.
func Capabilities() []Can {
	out := cli.Map(writers, func(w Writer) Can {
		return Can{Name: w.Name, Kind: "write", Provides: w.Provides,
			Produces: w.Produces.Name, Cost: "<1ms", Installed: true}
	})
	return append(out, cli.Map(checkers, func(ch Checker) Can {
		_, err := exec.LookPath(ch.Name)
		return Can{Name: ch.Name, Kind: "check", Provides: ch.Provides,
			Cost: ch.Cost, Needs: ch.Pin, Installed: err == nil}
	})...)
}

// runList is `dev seo can`.
func runList(c cli.Call) error {
	can := Capabilities()
	if c.WantsJSON() {
		return c.EmitJSON(can)
	}
	for _, e := range can {
		state := "ready"
		if !e.Installed {
			state = "NOT INSTALLED"
		}
		fmt.Fprintf(c.Stdout, "%-12s %-6s %s\n", e.Name, e.Kind, state)
		fmt.Fprintf(c.Stdout, "    gives   %s\n", e.Provides)
		if e.Produces != "" {
			fmt.Fprintf(c.Stdout, "    writes  %s\n", e.Produces)
		}
		if e.Needs != "" {
			fmt.Fprintf(c.Stdout, "    needs   mise.toml [tools] %s\n", e.Needs)
		}
		if e.Cost != "" {
			fmt.Fprintf(c.Stdout, "    costs   %s\n", e.Cost)
		}
	}
	return nil
}
