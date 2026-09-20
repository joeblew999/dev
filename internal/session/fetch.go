package session

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// copyTar takes every file under prefix out of archive and files it under as,
// reporting whether the prefix matched anything at all. It does not say what a
// miss means: only the caller knows which pin asked, and an error that names
// the pin is the one a reader can act on.
func copyTar(files skillFiles, archive []byte, prefix, as string) (found bool, err error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return false, err
	}
	defer gz.Close()
	r := tar.NewReader(gz)
	for {
		header, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false, err
		}
		if header.Typeflag != tar.TypeReg || !strings.HasPrefix(header.Name, prefix) {
			continue
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return false, err
		}
		files[path.Join(as, strings.TrimPrefix(header.Name, prefix))] = data
		found = true
	}
	return found, nil
}

func download(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// readDir reads a skills directory; a missing directory is an empty set.
// Symlinks mise made (packslip skills) and mise's own state file are skipped:
// mise owns those, not sync.
func readDir(dir string) (skillFiles, error) {
	files := skillFiles{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			if p != dir {
				if isSymlink(p) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if isSymlink(p) {
			return nil
		}
		// Not sync's files: mise's own state, and the record of what a real
		// session is allowed, which only verify writes.
		if base := filepath.Base(p); base == ".mise-skills.json" || base == sessionLockFile {
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

// isSymlink reports whether p itself is a symlink, without following it.
func isSymlink(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}
