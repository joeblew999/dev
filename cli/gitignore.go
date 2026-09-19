// This file keeps a repo's .gitignore naming what this package and the
// commands built on it write into. Whatever decides where something lands is
// what makes git ignore it, so a repo gets the line by building rather than
// by someone remembering to edit a file.
//
// It moved here from internal/ when the skills mirror needed it: mirror
// writes links into .agents/skills, so mirror has to ignore them, and two
// copies of this logic is the thing this repo keeps deleting.
package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// Ensure adds each entry to root/.gitignore that no line there already
// covers, creating the file when it is missing. It is idempotent: with
// nothing to add it does not touch the file, so a build is not a write.
func Ignore(root string, entries ...string) error {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	have := map[string]bool{}
	for _, line := range Lines(string(data)) {
		if p := pattern(line); p != "" {
			have[p] = true
		}
	}
	var add []string
	for _, e := range entries {
		if p := pattern(e); p != "" && !have[p] {
			have[p] = true
			add = append(add, e)
		}
	}
	if len(add) == 0 {
		return nil
	}
	var b strings.Builder
	b.Write(data)
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		b.WriteString("\n")
	}
	b.WriteString("# written per developer by the tool, so it keeps them out of git\n")
	for _, e := range add {
		b.WriteString(e + "\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// pattern is the directory a .gitignore line names, with the anchoring and
// trailing slash removed, so ".bin", "/.bin/" and ".bin/" are one answer and
// an upgrade does not append a line the repo already has in another shape.
func pattern(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	return strings.Trim(line, "/")
}
