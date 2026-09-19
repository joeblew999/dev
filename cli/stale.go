package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A command renders its manual from prose go:embed compiled into it, so what
// it writes is only as current as the binary. A binary built before the last
// edit renders the old prose — and says it succeeded, because from inside it
// nothing is wrong. `<cmd> skill` then rewrites every copy from stale bytes
// and reports "wrote ...", and `--check` compares that stale render against
// equally stale files and reports "up to date". Both are wrong, and neither
// can be seen from the output.
//
// That is what staleBuild is for: before a command writes or checks a manual,
// it asks whether anything it was built from has changed since. If so it
// refuses, rather than answering from bytes it knows are old.
//
// Only go test catches this otherwise, by recompiling; the guard exists so
// that the two commands a person actually runs cannot lie to them.

// sourceExts are the files that can change what a command renders: Go, and
// the markdown go:embed compiles in.
var sourceExts = map[string]bool{".go": true, ".md": true}

// notSource are directories holding build output, or documents no build
// reads. The list is deliberately short: naming one too few costs a needless
// rebuild, naming one too many costs a wrong answer, so the check errs
// towards refusing.
var notSource = map[string]bool{
	".git": true, ".bin": true, ".dist": true, "node_modules": true,
	ShippedDir: true, ".claude": true, ".agents": true, ".plans": true,
}

// notSourceFile is a file this command writes rather than reads. A manual's
// three copies are excluded by their directories above; README.md is not in
// one, and `<cmd> skill` writes its install block — so without this, writing
// the block makes the binary stale against its own output and the next run
// refuses. A file a command generates cannot be a file it is behind.
var notSourceFile = map[string]bool{"README.md": true}

// staleBuild names a file this command was built from that has changed since,
// or "" when the binary is current — or when the question does not apply.
//
// It applies only to a binary inside the repo, which is one this repo built.
// A release installed by mise lives outside it and carries prose fixed at the
// version pinned, which nothing in a consumer's repo can make stale; asking
// there would refuse every run for no reason.
func staleBuild(root string) string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return ""
	}
	if !under(root, exe) {
		return ""
	}
	built, err := os.Stat(exe)
	if err != nil {
		return ""
	}
	return newestAfter(root, built.ModTime())
}

// newestAfter is the first source under root modified after built, or "".
// It takes the time rather than reading the executable so that a test can
// ask the question without being able to move its own binary.
func newestAfter(root string, built time.Time) string {
	var newer string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if notSource[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !sourceExts[filepath.Ext(p)] || notSourceFile[d.Name()] {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(built) {
			newer = p
			return filepath.SkipAll
		}
		return nil
	})
	return newer
}

// under reports whether p is inside dir.
func under(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
