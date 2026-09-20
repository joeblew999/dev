package cloudflare

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/joeblew999/dev/internal/fnox"
	"github.com/joeblew999/dev/internal/suffix"
)

func TestDeleteTakesTheWorkerAndOnlyWhatWranglerProvisioned(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("wrangler.toml", []byte("name = \"api\"\nmain = \"x.mjs\"\nkv_namespaces = [{ binding = \"SOME_STORE\" }]\n[env.tinygo]\nkv_namespaces = [{ binding = \"SOME_STORE\" }]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var ran []string
	old := fnox.Run
	fnox.Run = func(u fnox.Under) (string, error) {
		line := strings.Join(u.Args, " ")
		ran = append(ran, line)
		if strings.HasPrefix(line, "wrangler kv namespace list") {
			out := " ⛅️ wrangler 4.131.1\n[{\"id\":\"1\",\"title\":\"api-some-store\"},{\"id\":\"2\",\"title\":\"SOME_STORE\"},{\"id\":\"3\",\"title\":\"api-alice-some-store\"}]\n"
			if u.Out != nil {
				io.WriteString(u.Out, out)
			}
			return out, nil
		}
		return "", nil
	}
	t.Cleanup(func() { fnox.Run = old })

	var out bytes.Buffer
	if err := Delete(strings.NewReader("y\n"), &out, ".", "", "", false); err != nil {
		t.Fatal(err)
	}
	want := []string{"wrangler kv namespace list", "wrangler delete --name api --force", "wrangler kv namespace delete --namespace-id 1"}
	if strings.Join(ran, "; ") != strings.Join(want, "; ") {
		t.Errorf("ran %q\nwant %q", ran, want)
	}
	if strings.Contains(out.String(), "SOME_STORE (2)") {
		t.Error("a namespace made by hand was slated for deletion")
	}

	// A developer's suffixed copy takes only its own namespace.
	ran = nil
	t.Setenv(suffix.Env, "alice")
	if err := Delete(nil, &out, ".", "", "", true); err != nil {
		t.Fatal(err)
	}
	if s := strings.Join(ran, "; "); !strings.Contains(s, "--name api-alice --force; wrangler kv namespace delete --namespace-id 3") || strings.Contains(s, "namespace-id 1") {
		t.Errorf("suffixed delete ran %q", ran)
	}

	// An explicit name is for what no config names any more.
	ran = nil
	if err := Delete(nil, &out, ".", "", "old-name", true); err != nil {
		t.Fatal(err)
	}
	if s := strings.Join(ran, "; "); !strings.Contains(s, "--name old-name --force") || strings.Contains(s, "namespace delete") {
		t.Errorf("named delete ran %q", ran)
	}

	// No answer means no.
	ran = nil
	if err := Delete(strings.NewReader("\n"), &out, ".", "", "", false); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("an unanswered question deleted: %v", err)
	}
	if len(ran) != 1 {
		t.Errorf("a refused delete still ran %q", ran)
	}
}
