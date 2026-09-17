// Package gitrepo answers the one question several verbs ask of the repo
// they run in: which GitHub repository is this, from the origin remote. gh
// is always told, since a clone with a second remote (a fork's upstream)
// makes it refuse to guess.
package gitrepo

import (
	"fmt"
	"os/exec"
	"strings"
)

// Slug is owner/repo from dir's origin remote.
func Slug(dir string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("no origin remote here; add one: git remote add origin https://github.com/<owner>/<repo>")
	}
	slug, ok := Parse(strings.TrimSpace(string(out)))
	if !ok {
		return "", fmt.Errorf("origin %q is not a GitHub repository URL", strings.TrimSpace(string(out)))
	}
	return slug, nil
}

// Parse is owner/repo from a GitHub remote URL in any of its spellings.
func Parse(url string) (string, bool) {
	url = strings.TrimSuffix(strings.TrimSuffix(url, "/"), ".git")
	for _, prefix := range []string{"https://github.com/", "http://github.com/", "git@github.com:", "ssh://git@github.com/"} {
		if rest, ok := strings.CutPrefix(url, prefix); ok && strings.Count(rest, "/") == 1 && !strings.HasPrefix(rest, "/") {
			return rest, true
		}
	}
	return "", false
}
