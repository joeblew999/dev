package vendored

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeblew999/dev/cli"
)

// Locked reads the file hashes sync recorded in the lock. The lock is part of
// the set, so every destination has one; the primary answers for all of them,
// and Diff is what proves the rest agree.
func Locked() (map[string]string, error) {
	lines, err := lockLines()
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	for _, line := range lines {
		name, rest, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		// A skill's source line ("tdd\tgithub.com/mattpocock/skills@c55e...")
		// is not a hash: a file line is 64 hex characters and nothing else.
		if hash := strings.TrimSpace(rest); len(hash) == 64 {
			files[name] = hash
		}
	}
	return files, nil
}

// LockedNames is every skill the lock records, which is what an agent should
// be able to load.
func LockedNames() ([]string, error) {
	data, err := os.ReadFile(LockPath(Primary()))
	if os.IsNotExist(err) {
		// A repo that pins only [claude] vendors no skills and has no lock.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := cli.Lines(string(data))
	return cli.Sorted(cli.Collect(lines, func(line string) (string, bool) {
		name, rest, ok := strings.Cut(line, "\t")
		// The stanza head, not one of its files: it names an upstream rather
		// than a hash, and it is the only line that names the skill itself.
		return name, ok && len(strings.TrimSpace(rest)) != 64 && !strings.Contains(name, "/")
	})), nil
}

// LockPath is where the lock lives in a given destination.
func LockPath(dir string) string { return filepath.Join(dir, LockFile) }

// lockLines reads the lock, saying how to make one when there is none.
func lockLines() ([]string, error) {
	data, err := os.ReadFile(LockPath(Primary()))
	if err != nil {
		return nil, fmt.Errorf("%w; there is no record of what was synced", err)
	}
	return cli.Lines(string(data)), nil
}
