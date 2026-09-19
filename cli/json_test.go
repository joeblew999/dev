package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The banner is the whole reason DecodeJSON exists: wrangler and flyctl both
// print one, and both packages had worked that out separately.
func TestDecodeJSONStartsAtTheJSON(t *testing.T) {
	type app struct {
		Name string `json:"name"`
	}
	for _, tc := range []struct {
		name, text, want string
	}{
		{"plain array", `[{"name":"a"}]`, "a"},
		{"after a banner", "⛅️ wrangler 4.0.0\n---\n[{\"name\":\"a\"}]", "a"},
		{"after a warning line", "warning: using default org\n[{\"name\":\"a\"}]\n", "a"},
	} {
		got, err := DecodeJSON[[]app]("a tool's list", tc.text)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if len(got) != 1 || got[0].Name != tc.want {
			t.Errorf("%s: got %v; want one app named %q", tc.name, got, tc.want)
		}
	}
	// An object is as common as an array, and a tool that says nothing at all
	// is the failure people actually hit — a missing binary, a silent error.
	type report struct {
		Score int `json:"score"`
	}
	if got, err := DecodeJSON[report]("a checker", "noise\n{\"score\":62}"); err != nil || got.Score != 62 {
		t.Errorf("object: got %v, %v; want score 62", got, err)
	}
	if _, err := DecodeJSON[report]("a checker", "   \n"); err == nil || !strings.Contains(err.Error(), "printed no JSON") {
		t.Errorf("empty output: err = %v; want it to say the tool printed no JSON", err)
	}
	// The error names the source, because whoever reads it is looking at the
	// output of several tools and does not yet know which one broke.
	if _, err := DecodeJSON[report]("scoutly's report", "{oops"); err == nil || !strings.Contains(err.Error(), "scoutly's report") {
		t.Errorf("bad JSON: err = %v; want it to name scoutly's report", err)
	}
}

// emitCall is one parsed invocation with the JSON flags on it.
func emitCall(t *testing.T, out io.Writer, args ...string) Call {
	t.Helper()
	fs := Flags("tool report", io.Discard)
	JSONFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return Call{Verb: "tool report", Flags: fs, Stdout: out, Stderr: io.Discard}
}

func TestWantsJSON(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"--json"}, true},
		{[]string{"--json=false"}, false},
		// Asking for a file is asking for JSON: nobody means --out as "write
		// the file but print prose".
		{[]string{"--out", "r.json"}, true},
	} {
		if got := emitCall(t, io.Discard, tc.args...).WantsJSON(); got != tc.want {
			t.Errorf("WantsJSON(%q) = %v; want %v", tc.args, got, tc.want)
		}
	}
}

func TestEmitJSONWritesStdoutAndTheFile(t *testing.T) {
	dir := t.TempDir()
	// A path under a directory that does not exist yet: a report usually goes
	// somewhere like .bin/reports, which nothing has made.
	path := filepath.Join(dir, "reports", "seo.json")
	var out bytes.Buffer
	c := emitCall(t, &out, "--out", path)
	if err := c.EmitJSON(map[string]any{"score": 62, "outcome": "gate"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(out.Bytes(), []byte("\n")) {
		t.Error("stdout has no trailing newline")
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, out.Bytes()) {
		t.Errorf("the file and stdout differ:\nfile:   %s\nstdout: %s", onDisk, out.Bytes())
	}
	var back map[string]any
	if err := json.Unmarshal(onDisk, &back); err != nil {
		t.Fatalf("what was written is not JSON: %v", err)
	}
	if back["outcome"] != "gate" {
		t.Errorf("read back %v; want outcome gate", back)
	}
}

// The shape a command that runs several tools takes: each tool decodes into
// its own type, they are merged into one report, and the report goes to
// stdout and to a file in one call. Held here because that is the promise
// made to anything built on cli — not to any one command.
func TestManyToolsMergeIntoOneReport(t *testing.T) {
	type crawl struct {
		Pages int      `json:"pages"`
		Bad   []string `json:"bad"`
	}
	type links struct {
		Checked int `json:"checked"`
		Broken  int `json:"broken"`
	}
	type report struct {
		URL   string `json:"url"`
		Crawl crawl  `json:"crawl"`
		Links links  `json:"links"`
	}

	// What each tool printed, banners and all.
	crawled, err := DecodeJSON[crawl]("the crawler", "crawling…\n{\"pages\":3,\"bad\":[\"/a\"]}")
	if err != nil {
		t.Fatal(err)
	}
	linked, err := DecodeJSON[links]("the link checker", "checked 12 links\n{\"checked\":12,\"broken\":1}")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	var out bytes.Buffer
	c := emitCall(t, &out, "--out", path)
	if err := c.EmitJSON(report{URL: "https://example.com", Crawl: crawled, Links: linked}); err != nil {
		t.Fatal(err)
	}

	var back report
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Crawl.Pages != 3 || back.Links.Broken != 1 || back.URL != "https://example.com" {
		t.Errorf("merged report read back as %+v", back)
	}
}
