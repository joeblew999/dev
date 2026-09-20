// Package vendored is the skill set on disk.
//
// Its one source of truth is the filesystem. It knows nothing about where a
// skill came from — not the repo it was taken out of, not the commit, not
// which preset asked for it. It is handed a set of files and it puts them
// where agents read them, or reads back what is there and says how it differs.
//
// There is one set and several destinations. An agent reads its own directory
// and no other, so a skill written to one alone is a skill the rest cannot
// see; writing every destination from one set is what makes that impossible
// rather than merely unlikely.
package vendored

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeblew999/dev/cli"
)

// LockFile records what sync wrote: one line naming each skill's upstream,
// then one line per file with its hash, so check can verify the files without
// downloading anything.
//
// It is not the other lock. SESSION.lock, beside it, records what a running
// Claude Code session reported it could load — a different question, answered
// by a different thing, and the two are kept apart by living in different
// packages rather than by the reader remembering which is which.
const LockFile = "SKILLS.lock"

// Set is a skill set: path relative to a skills directory -> contents.
type Set map[string][]byte

// Dirs are the directories this set is written to.
func Dirs() []string { return cli.AgentDirs() }

// Primary is the directory read back when one has to be chosen — reporting
// which skills exist, or finding the lock. They are written identically, so
// any of them would answer; naming one keeps the answer stable.
func Primary() string { return Dirs()[0] }

// Skipped are files inside a skills directory that sync does not own: mise's
// state, and the record of a live session, which only verify writes.
func skipped(base string) bool {
	return base == ".mise-skills.json" || base == "SESSION.lock"
}

// Write puts the set in every directory an agent reads, deleting only files a
// previous sync owned. Anything else is left alone, so a skill the repo wrote
// itself survives, and so does a symlink mise made.
func Write(set Set) error {
	for _, dir := range Dirs() {
		have, err := Read(dir)
		if err != nil {
			return err
		}
		if err := writeOwned(dir, have, set); err != nil {
			return err
		}
	}
	return nil
}

// Existing reports which destinations were already there, so a caller can say
// that a directory is new without guessing.
func Existing() []string {
	return cli.Filter(Dirs(), func(dir string) bool {
		info, err := os.Stat(dir)
		return err == nil && info.IsDir()
	})
}

// Read reads one skills directory; a missing directory is an empty set.
// Symlinks mise made are skipped: mise owns those, not sync.
func Read(dir string) (Set, error) {
	files := Set{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil && os.IsNotExist(err):
			return nil
		case err != nil:
			return err
		case isSymlink(p):
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		case d.IsDir(), skipped(filepath.Base(p)):
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	return files, err
}

// Diff describes how what is on disk differs from what the lock records, for
// every destination. A skill missing from one agent's directory is as much a
// difference as one whose contents changed.
func Diff(want map[string]string) ([]string, int, error) {
	var all []string
	for _, dir := range Dirs() {
		have, err := Read(dir)
		if err != nil {
			return nil, 0, err
		}
		for _, line := range diffLocked(have, want) {
			all = append(all, dir+": "+line)
		}
	}
	return all, len(want), nil
}

// isSymlink reports whether p itself is a symlink, without following it.
func isSymlink(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// writeOwned writes want into dir, deleting only files sync owned before.
// Owned paths come from the previous lock: every top-level skill directory it
// names, plus the lock file itself.
func writeOwned(dir string, have, want Set) error {
	owned := ownedPaths(have[LockFile])
	for name := range have {
		if _, keep := want[name]; keep || !isOwned(name, owned) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			return err
		}
	}
	for name, data := range want {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			return err
		}
	}
	removeEmptyDirs(dir)
	return nil
}

// ownedPaths returns the top-level paths the previous sync owned: the lock
// file and every skill directory it names.
func ownedPaths(lock []byte) []string {
	owned := []string{LockFile}
	for _, line := range cli.Lines(string(lock)) {
		if name, _, ok := strings.Cut(line, "\t"); ok && name != "" {
			owned = append(owned, name)
		}
	}
	return owned
}

func isOwned(name string, owned []string) bool {
	for _, prefix := range owned {
		if name == prefix || strings.HasPrefix(name, prefix+"/") {
			return true
		}
	}
	return false
}

// removeEmptyDirs deletes directories left empty after owned files moved out,
// so a dropped skill leaves no empty folder behind.
func removeEmptyDirs(dir string) {
	var dirs []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != dir {
			dirs = append(dirs, p)
		}
		return nil
	})
	// Deepest first, so a directory is removed after what it holds.
	for _, d := range cli.SortedDesc(dirs) {
		_ = os.Remove(d)
	}
}

// diffFiles describes how one set differs from another, most useful first.
func diffFiles(have, want Set) []string { return cli.DiffMaps(have, want, bytes.Equal) }

// Same reports whether two sets hold the same files with the same contents.
func Same(a, b Set) bool { return len(diffFiles(a, b)) == 0 }

// Hash is how the lock records a file.
func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// LockSkill is one skill's stanza: where it came from, then every file it
// brought with its hash.
func LockSkill(name, from string, files Set) string {
	lines := []string{fmt.Sprintf("%s\t%s", name, from)}
	prefix := name + "/"
	for _, file := range cli.Filter(SortedFiles(files), func(f string) bool {
		return f == name || strings.HasPrefix(f, prefix)
	}) {
		lines = append(lines, fmt.Sprintf("%s\t%s", file, Hash(files[file])))
	}
	return strings.Join(lines, "\n")
}

// SortedFiles is every file path a lock covers, in order: the lock itself is
// not one of them, because sync rewrites it and its hash can never match. One
// place says that, so a caller cannot forget it.
func SortedFiles(files Set) []string {
	return cli.Sorted(cli.Filter(cli.SortedKeys(files), func(name string) bool { return name != LockFile }))
}

// diffLocked describes how the files on disk differ from the hashes recorded.
func diffLocked(have Set, want map[string]string) []string {
	return cli.DiffMaps(hashes(have), want, func(a, b string) bool { return a == b })
}

// hashes is a set reduced to what the lock compares.
func hashes(files Set) map[string]string {
	out := map[string]string{}
	for _, name := range SortedFiles(files) {
		out[name] = Hash(files[name])
	}
	return out
}

// Remove takes back everything a previous sync owned, in every destination,
// and reports the skills that went. Anything sync did not own is left exactly
// as it was: a skill the repo wrote itself, a symlink mise made, a file
// somebody dropped in by hand.
//
// It is Write with nothing wanted, which is the whole of what "undo" means
// here — the ownership rule that keeps sync from trampling a repo's own files
// is the same rule that lets it take back only its own.
func Remove() ([]string, error) {
	went, err := LockedNames()
	if err != nil {
		return nil, err
	}
	for _, dir := range Dirs() {
		have, err := Read(dir)
		if err != nil {
			return nil, err
		}
		if err := writeOwned(dir, have, Set{}); err != nil {
			return nil, err
		}
	}
	return went, nil
}
