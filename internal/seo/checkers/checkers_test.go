// Every adapter, against output the tool really printed.
//
// The fixtures in testdata are what each checker wrote about a live site,
// captured from a run rather than composed — which is the only way to catch
// the mistakes that actually happen here: a field that is named something
// else, a value that is an array where a string was expected, a verdict that
// is not on the last line.
//
// They are frozen, so they cannot tell us a tool has changed its output.
// What they hold is our reading of it, which is the half we own.
package checkers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
)

// recorded is what a checker printed, as a Result.
func recorded(t *testing.T, name string) tool.Result {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	// A checker prints its progress first, and the decode skips it: the
	// fixture goes through everything the tool's own output does.
	return tool.Result{Bin: name, Out: "working…\n" + string(data), Took: time.Second}
}

// Every checker reads its own output into findings that carry what a report
// needs: an id, a severity a gate understands, and a fix.
func TestEveryCheckerReadsItsOwnOutput(t *testing.T) {
	for _, ch := range All {
		t.Run(ch.Name, func(t *testing.T) {
			found, err := ch.Read(recorded(t, ch.Name))
			if err != nil {
				t.Fatalf("reading what %s printed: %v", ch.Name, err)
			}
			if found.Covered() == "" {
				t.Errorf("%s says nothing about what it looked at", ch.Name)
			}
			for _, f := range found.Issues {
				switch {
				case f.Tool != ch.Name:
					t.Errorf("a finding is attributed to %q, not %q", f.Tool, ch.Name)
				case f.ID == "":
					t.Errorf("a finding has no id, so no CI rule can match it: %+v", f)
				case f.Message == "":
					t.Errorf("%s has no message", f.ID)
				case f.Fix == "":
					t.Errorf("%s has no fix, which is the half a reader acts on", f.ID)
				case cli.SevRank(f.Severity) > cli.SevRank(cli.SevInfo):
					t.Errorf("%s has severity %q, which no gate understands", f.ID, f.Severity)
				}
			}
		})
	}
}

// What each one is for, on the site the fixtures came from. These are the
// findings that made the tool worth adding, so they are the ones worth
// holding: if a change stops kitsune reporting a missing canonical, the
// report is poorer and nothing else would say so.
func TestEachCheckerFindsWhatItIsFor(t *testing.T) {
	for _, tc := range []struct {
		checker string
		want    string
	}{
		{"kitsune", "seo.canonical.missing"},    // the precise dotted ids
		{"scoutly", "missing-meta-description"}, // crawl and on-page
		{"scry", "security/"},                   // the broad sweep, headers included
		{"muffet", "broken-link"},               // the link layer
		{"seo-audit", "CANONICAL"},              // its own SHOUTING id, unrewritten
		{"ldlint", "missing-json-ld"},           // the schema.org vocabulary
	} {
		ch := byName(t, tc.checker)
		found, err := ch.Read(recorded(t, ch.Name))
		if err != nil {
			t.Fatal(err)
		}
		ids := strings.Join(cli.Map(found.Issues, func(f cli.Finding) string { return f.ID }), " ")
		if !strings.Contains(ids, tc.want) {
			t.Errorf("%s did not report %q, which is what it is here for; it said: %s",
				tc.checker, tc.want, cli.Or(ids, "nothing"))
		}
	}
}

// A checker that prints nothing useful says so rather than inventing an
// answer, and one whose output is not what it should be says which tool.
func TestBadOutputNamesTheTool(t *testing.T) {
	for _, ch := range All {
		if _, err := ch.Read(tool.Result{Bin: ch.Name, Out: "{oh no"}); err != nil {
			if !strings.Contains(err.Error(), ch.Name) && !strings.Contains(err.Error(), "report") {
				t.Errorf("%s: %v; want an error naming whose output it was", ch.Name, err)
			}
		}
	}
}

// icanhasrobot prints its verdict and then, with no rules at all, a notice
// explaining why — so the verdict is the end of a line and not the end of
// the output. Reading the last line called that no verdict, which is the
// bug this holds.
func TestRobotsVerdictIsFoundByPatternNotPosition(t *testing.T) {
	robots := byName(t, "icanhasrobot")
	for _, tc := range []struct {
		name, out, want string
	}{
		{"allowed", "user-agent 'Googlebot' with URI 'https://x/': ALLOWED\n", ""},
		{"blocked", "user-agent 'Googlebot' with URI 'https://x/a': DISALLOWED\n", "blocked-by-robots"},
		{"no rules at all", "user-agent 'Googlebot' with URI 'https://x/': ALLOWED\n" +
			"notice: robots file is empty so all user-agents are allowed\n", "robots-missing"},
	} {
		found, err := robots.Read(tool.Result{Bin: "icanhasrobot", Out: tc.out})
		if err != nil {
			t.Fatal(err)
		}
		ids := strings.Join(cli.Map(found.Issues, func(f cli.Finding) string { return f.ID }), " ")
		if ids != tc.want {
			t.Errorf("%s: got %q; want %q", tc.name, ids, tc.want)
		}
	}
}

// Paged is how both crawlers are told how far to go, and zero means the tool
// decides rather than being handed a nonsense limit.
func TestPagedOnlyWhenAsked(t *testing.T) {
	if got := Paged([]string{"x"}, 0); len(got) != 1 {
		t.Errorf("Paged with no limit = %v; want the arguments untouched", got)
	}
	if got := Paged([]string{"x"}, 5); strings.Join(got, " ") != "x --max-pages 5" {
		t.Errorf("Paged = %v", got)
	}
}

// Every fault a checker can name has advice, or the report tells a reader
// what is wrong and not what to do.
func TestEveryKnownCodeHasAdvice(t *testing.T) {
	for code, fix := range fixes {
		if !strings.Contains(fix, "https://developers.google.com/") {
			t.Errorf("%s: the advice names no Google page: %q", code, fix)
		}
	}
	// A code nobody wrote advice for still points somewhere.
	if fix := Fix("something-invented-tomorrow"); !strings.Contains(fix, DocEssentials) {
		t.Errorf("an unknown code got %q", fix)
	}
}

func byName(t *testing.T, name string) Checker {
	t.Helper()
	for _, ch := range All {
		if ch.Name == name {
			return ch
		}
	}
	t.Fatalf("no checker named %q", name)
	return Checker{}
}
