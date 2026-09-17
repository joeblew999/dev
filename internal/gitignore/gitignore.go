// Package gitignore keeps a repo's .gitignore naming the directories dev
// writes into. dev decides where build output goes, so dev is what makes git
// ignore it: a repo that upgrades gets the line by building, not by someone
// remembering to edit a file.
package gitignore

import (
	"os"
	"path/filepath"
	"strings"
)

// Ensure adds each entry to root/.gitignore that no line there already
// covers, creating the file when it is missing. It is idempotent: with
// nothing to add it does not touch the file, so a build is not a write.
func Ensure(root string, entries ...string) error {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	have := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
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
	b.WriteString("# build output: dev writes here, so dev keeps it out of git\n")
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
