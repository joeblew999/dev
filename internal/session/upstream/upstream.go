// Package upstream is what a repository holds at one commit.
//
// Its one source of truth is the network. It knows nothing about this repo:
// not session.toml, not .claude/skills, not what anyone intends to do with
// what it hands back. Give it an owner/name and a commit and it can answer
// questions about that snapshot and nothing else.
//
// That boundary is why it can be trusted about the half it owns. A wrong
// answer here means GitHub served something unexpected, never that a local
// file was misread.
package upstream

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"
)

// PluginManifest is where Claude Code plugins declare themselves. An upstream
// that ships as a plugin puts one here, and it is the repository's own
// statement about its skills — which beats guessing from directory names.
const PluginManifest = ".claude-plugin/plugin.json"

// Archive is one repository at one commit, decompressed once. Every file is
// held by its path below the top directory, so asking about several skills and
// the manifest costs one download and one decompression rather than one of
// each per question.
type Archive struct {
	Repo, Ref string
	files     map[string][]byte
}

// Plugin is what .claude-plugin/plugin.json says. Skills is the repository's
// own list of where its skills are, relative to the repository root, and is
// the authority when it is there: mattpocock's lists twenty-five paths under
// two categories and leaves out the ones filed as deprecated or in progress,
// which no directory listing would tell apart.
//
// An upstream may ship a plugin with no such list — superpowers does — and
// then there is nothing to be authoritative with, and convention decides.
type Plugin struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Skills      []string `json:"skills"`
}

// Get downloads a repository at a commit. codeload names the top directory
// after the repository, which is the only place that name is needed — Archive
// strips it, so nothing downstream has to know the convention.
func Get(repo, ref string) (*Archive, error) {
	if strings.Count(repo, "/") != 1 {
		return nil, fmt.Errorf("repo %q wants owner/name", repo)
	}
	data, err := download(fmt.Sprintf("https://codeload.github.com/%s/tar.gz/%s", repo, ref))
	if err != nil {
		return nil, err
	}
	top := repo[strings.LastIndex(repo, "/")+1:] + "-" + ref + "/"
	files, err := untar(data, top)
	if err != nil {
		return nil, fmt.Errorf("%s@%s: %w", repo, ref, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s@%s: the archive holds nothing under %s", repo, ref, top)
	}
	return &Archive{Repo: repo, Ref: ref, files: files}, nil
}

// Plugin reads the repository's plugin manifest, and reports whether it ships
// as one at all. A repository that is not a plugin is not an error: plenty of
// upstreams are a skills directory and nothing more.
func (a *Archive) Plugin() (Plugin, bool, error) {
	return a.JSON[Plugin](PluginManifest)
}

// JSON reads one file from the archive into whatever shape the caller wants,
// and reports whether the repository has that file at all.
//
// A method with its own type parameter, which Go has had since 1.27 — the
// same shape as tool.Result.JSON. Written as a package function first, on the
// belief that a method could not introduce a type parameter beyond the
// receiver's, which was true of the Go this was learned from and is not true
// of the Go this repo pins.
func (a *Archive) JSON[T any](name string) (T, bool, error) {
	var out T
	data, ok := a.files[name]
	if !ok {
		return out, false, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, true, fmt.Errorf("%s@%s: %s is not the JSON it should be: %w", a.Repo, a.Ref, name, err)
	}
	return out, true, nil
}

// Dir copies every file under one directory into out, keyed under as, and
// reports whether the directory held anything. It does not say what a miss
// means: only the caller knows which pin asked, and an error that names the
// pin is the one a reader can act on.
func (a *Archive) Dir(out map[string][]byte, within, as string) (found bool) {
	prefix := strings.Trim(within, "/") + "/"
	for name, data := range a.files {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		out[path.Join(as, strings.TrimPrefix(name, prefix))] = data
		found = true
	}
	return found
}

// Has reports whether one path in the archive is a directory holding files.
func (a *Archive) Has(within string) bool {
	prefix := strings.Trim(within, "/") + "/"
	for name := range a.files {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// untar decompresses an archive into path -> contents, with top stripped.
func untar(archive []byte, top string) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	files := map[string][]byte{}
	r := tar.NewReader(gz)
	for {
		header, err := r.Next()
		if errors.Is(err, io.EOF) {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg || !strings.HasPrefix(header.Name, top) {
			continue
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		files[strings.TrimPrefix(header.Name, top)] = data
	}
}

// downloadClient is the one this package fetches through.
//
// http.Get uses http.DefaultClient, which has no timeout at all: a GitHub
// tarball from a host that accepts the connection and then says nothing
// hangs a sync forever, with no way to interrupt it but killing the process.
// Every other client in this tree sets one; this was the site that drifted,
// and nothing said so until a linter did.
//
// Two minutes rather than the thirty seconds the others use, because this
// one is downloading an archive rather than asking a question.
var downloadClient = &http.Client{Timeout: 2 * time.Minute}

func download(url string) ([]byte, error) {
	resp, err := downloadClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// SkillFile is what makes a directory a skill. An agent loads a directory
// because this is in it, so the set of directories holding one is the set of
// skills a repository has — whatever it calls its folders, whatever it lists
// in a manifest, and whether or not it ships as a plugin at all.
const SkillFile = "SKILL.md"

// Skills is every skill in the repository, as the path it sits at. It is
// derived from the files rather than from anything the repository says about
// itself, so it works for an upstream with no manifest, one whose manifest
// lists nothing, and one that files skills somewhere nobody guessed.
func (a *Archive) Skills() []string {
	var found []string
	for name := range a.files {
		if dir, file := path.Split(name); file == SkillFile {
			found = append(found, strings.TrimSuffix(dir, "/"))
		}
	}
	sort.Strings(found)
	return found
}

// Find is where a skill of this name sits, searching what the repository
// declares first and then what its files show. A name that is already a path
// to a skill is taken at its word.
func (a *Archive) Find(name string) (string, bool) {
	want := path.Base(strings.Trim(name, "/"))
	if a.Has(name) {
		return strings.Trim(name, "/"), true
	}
	plugin, isPlugin, err := a.Plugin()
	if err == nil && isPlugin {
		for _, declared := range plugin.Skills {
			at := strings.TrimPrefix(strings.Trim(declared, "/"), "./")
			if path.Base(at) == want {
				return at, true
			}
		}
	}
	for _, at := range a.Skills() {
		if path.Base(at) == want {
			return at, true
		}
	}
	return "", false
}
