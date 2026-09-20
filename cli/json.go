// JSON in and out: reading what another tool printed, and answering in the
// one shape a machine can read. Both sides of that were about to be written
// per package — the reading already had been, twice, identically — and a
// report that differs in shape per verb is a report nothing can consume.
package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DecodeJSON reads a T out of what a tool printed.
//
// Decoding starts at the first [ or { rather than at byte zero, because a CLI
// prints what it likes first: wrangler opens with a banner, flyctl with a
// version notice, and a tool run under a wrapper may add a line of its own.
// Two packages had worked that out separately and written the same three
// lines; a package that runs several tools would have written it once per
// tool.
//
// What names the source in the error — "wrangler's namespace list" — because
// a decode failure is read by someone who does not yet know which of several
// tools produced the text.
func DecodeJSON[T any](what, text string) (T, error) {
	var v T
	if i := strings.IndexAny(text, "[{"); i >= 0 {
		text = text[i:]
	}
	if strings.TrimSpace(text) == "" {
		return v, fmt.Errorf("reading %s: it printed no JSON", what)
	}
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return v, fmt.Errorf("reading %s: %w", what, err)
	}
	return v, nil
}

// JSONFlags are the two flags a verb needs to answer in JSON rather than in
// prose: --json prints it instead of the human text, and --out PATH writes
// the same bytes to a file. A verb that reports anything a machine might read
// registers these rather than inventing its own spelling, so every command on
// the stack is asked the same way.
func JSONFlags(fs *flag.FlagSet) {
	fs.Var(new(Bool), "json", "print the report as JSON instead of text")
	fs.String("out", "", "also write the JSON report to `PATH`, or to .reports/ when given a bare name")
}

// WantsJSON reports whether this call was asked for JSON. Asking for a file
// is asking for JSON: --out with no --json is a request nobody means as
// "write the file but print prose".
func (c Call) WantsJSON() bool { return c.Given("json") || c.Value("out") != "" }

// ReportsDir is where a report goes when --out names a file rather than a
// path: one directory at the repo root, so a person, a task and an agent all
// look in the same place for what the last run found. Gitignored, and by the
// code that writes it — a report is this clone's, like .bin and the skill
// links beside it.
const ReportsDir = ".reports"

// reportPath resolves what --out was given. A path with a separator in it is
// used as written, because someone who typed one means it. A bare name lands
// in ReportsDir, which is made and ignored on the way.
func reportPath(out string) (string, error) {
	if strings.ContainsRune(out, filepath.Separator) || filepath.IsAbs(out) {
		return out, nil
	}
	root, err := root()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, ReportsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := Ignore(root, ReportsDir+"/"); err != nil {
		return "", err
	}
	return filepath.Join(dir, out), nil
}

// EmitJSON writes v as indented JSON to Stdout, and to --out as well when a
// path was given. Indented because both readers are people some of the time,
// and a trailing newline because a terminal needs one and a file is no worse
// for it.
//
// Stdout carries the data and nothing else — progress and warnings belong on
// Stderr — so a task may pipe one verb's report into another program without
// anything having to be stripped first.
func (c Call) EmitJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: writing the report as JSON: %w", c.Verb, err)
	}
	data = append(data, '\n')
	if out := c.Value("out"); out != "" {
		path, err := reportPath(out)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Verb, err)
		}
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("%s: making %s for --out: %w", c.Verb, dir, err)
			}
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("%s: writing %s: %w", c.Verb, path, err)
		}
	}
	_, err = c.Stdout.Write(data)
	return err
}

// SubReport writes what one tool printed, beside the main report, and returns
// the path to name in it.
//
// Tools do not agree on a shape and should not be made to: each says what it
// found in its own JSON, and forcing them through one struct would throw away
// everything the merged report does not model. So the merged report carries
// what every tool has in common, and links to each tool's own output for the
// rest — a person follows the link, and so does an agent.
//
// They land in a directory named after the main report, so one run's outputs
// stay together: --out seo.json puts them in .reports/seo/<tool>.json.
// Without --out there is no main report to sit beside, and nothing is written.
func (c Call) SubReport(tool, raw string) (string, error) {
	out := c.Value("out")
	if out == "" || raw == "" {
		return "", nil
	}
	main, err := reportPath(out)
	if err != nil {
		return "", err
	}
	dir := strings.TrimSuffix(main, filepath.Ext(main))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("%s: making %s: %w", c.Verb, dir, err)
	}
	// Indented when it is JSON, verbatim when it is not. Tools disagree about
	// whitespace — one pretty-prints, the next emits a single line a thousand
	// characters long — and a report nobody can read is a report nobody
	// reads. Whitespace is the only thing this changes: every byte of data
	// the tool produced is still there, which is what keeping it is for. It
	// also makes a diff between two runs line-by-line rather than one line.
	data := []byte(raw)
	var pretty bytes.Buffer
	if json.Indent(&pretty, data, "", "  ") == nil {
		data = append(pretty.Bytes(), '\n')
	}
	path := filepath.Join(dir, tool+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("%s: writing %s: %w", c.Verb, path, err)
	}
	// Relative to the repo, because the path is read in a report rather than
	// followed from wherever the reader's shell happens to be.
	if root, err := root(); err == nil {
		if rel, err := filepath.Rel(root, path); err == nil {
			return rel, nil
		}
	}
	return path, nil
}
