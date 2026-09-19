package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeblew999/dev/cli"
)

// lockedFiles reads the file hashes sync recorded in the lock.
func lockedFiles() (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(skillsDir, lockFile))
	if err != nil {
		return nil, fmt.Errorf("%w; run: "+syncCmd, err)
	}
	files := map[string]string{}
	for _, line := range cli.Lines(string(data)) {
		name, rest, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		hash := strings.TrimSpace(rest)
		// A skill's source line ("gsx\tgithub.com/gsxhq/gsx@v0.1.1") is not
		// a hash: it has slashes and an @. A file line is 64 hex chars.
		if len(hash) != 64 {
			continue
		}
		files[name] = hash
	}
	return files, nil
}

// diffLocked compares files on disk against the lock hashes. The lock itself
// is skipped: sync rewrites it, so its hash can never match.
func diffLocked(have skillFiles, want map[string]string) []string {
	return cli.DiffMaps(have, want, func(data []byte, hash string) bool { return hashFile(data) == hash }, lockFile)
}

func hashFile(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// lockSkill formats a skill's lock lines: its source, then one hash per file
// under its directory. prefix matches only this skill's own files, so an
// earlier skill's files never leak into a later skill's lines.
func lockSkill(name, source string, files skillFiles) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("%s\t%s", name, source))
	prefix := name + "/"
	names := cli.Filter(sortedFiles(files), func(file string) bool {
		return file == name || strings.HasPrefix(file, prefix)
	})
	for _, file := range names {
		lines = append(lines, fmt.Sprintf("%s\t%s", file, hashFile(files[file])))
	}
	return strings.Join(lines, "\n")
}

// sortedFiles is every file path a lock covers, in order: the lock itself is
// not one of them, because sync rewrites it and its hash can never match. One
// place says that, so a caller cannot forget it.
func sortedFiles(files skillFiles) []string {
	return cli.Without(cli.SortedKeys(files), lockFile)
}
