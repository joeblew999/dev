// The MCP servers are part of the session: .mcp.json declaring one proves
// nothing, four were shipped once that all sat at "Needs authentication",
// which no fresh clone could use. `dev session mcp` asks each to connect.

package session

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/joeblew999/dev/cli"

	"github.com/joeblew999/dev/cli/tool"
)

// MCP runs `claude mcp list` and fails on any server that does not connect.
func MCP(stdout, stderr io.Writer) error {
	res, err := tool.Cmd{Bin: ClaudeBin, Pin: claudePin, Args: []string{"mcp", "list"}, Combined: true}.Capture()
	out := res.Out
	fmt.Fprint(stdout, out)
	if err != nil && len(out) == 0 {
		return fmt.Errorf("claude mcp list: %w (is Claude Code installed?)", err)
	}
	if bad := Unusable(string(out)); len(bad) > 0 {
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "A declared MCP server is not usable on a fresh clone:")
		for _, b := range bad {
			fmt.Fprintln(stderr, "  "+b)
		}
		fmt.Fprintln(stderr, "Either it needs a per-developer login, in which case drop it from .mcp.json,")
		fmt.Fprintln(stderr, "or it is misconfigured. Fix .mcp.json, then: mise run session:mcp")
		return fmt.Errorf("%d MCP server(s) not usable", len(bad))
	}
	return nil
}

var unusable = regexp.MustCompile(`(?i)needs authentication|failed to connect|✗`)

// Unusable returns the lines of a `claude mcp list` report naming a server
// that a fresh clone could not use.
func Unusable(report string) []string {
	return cli.Collect(strings.Split(report, "\n"), func(line string) (string, bool) {
		if unusable.MatchString(line) {
			return strings.TrimSpace(line), true
		}
		return "", false
	})
}
