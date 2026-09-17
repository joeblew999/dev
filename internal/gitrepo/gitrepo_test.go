package gitrepo

import "testing"

func TestParseEverySpelling(t *testing.T) {
	for url, want := range map[string]string{
		"https://github.com/acme/tool.git":   "acme/tool",
		"https://github.com/acme/tool":       "acme/tool",
		"git@github.com:acme/tool.git":       "acme/tool",
		"ssh://git@github.com/acme/tool":     "acme/tool",
		"https://gitlab.com/acme/tool":       "",
		"https://github.com/acme/tool/extra": "",
		"https://github.com/acme":            "",
	} {
		got, ok := Parse(url)
		if got != want || ok != (want != "") {
			t.Errorf("Parse(%q) = %q, %v; want %q", url, got, ok, want)
		}
	}
}
