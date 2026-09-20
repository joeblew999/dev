// What the writers promise each other.
//
// A writer's output is often a claim about a file another writer produces,
// and that is the one thing about this registry nothing else can check: the
// artifacts are written to a directory and read back by their own validators,
// which say whether each file is good and never whether the set is coherent.
package seo

import (
	"strings"
	"testing"

	"github.com/joeblew999/dev/cli"
)

// The chain the writers form, held here because nothing else can see it: a
// writer's output makes a claim about a file another writer produces, and
// until Needs existed that claim was true only because the registry happened
// to list sitemap first. `--only robots` wrote a robots.txt naming a
// sitemap.xml that was never written, and reported pass.
func TestEveryNeedNamesAnArtifactAWriterProduces(t *testing.T) {
	for _, w := range writers {
		for _, need := range w.Needs {
			if _, ok := producer(need); !ok {
				t.Errorf("%s needs %q and no writer produces it", w.Name, need)
			}
		}
	}
}

// Nothing runs before what it depends on. The order is derived from Needs
// rather than kept in the registry literal, so this also catches a cycle:
// ordered() breaks one by running the writer anyway, and then a dependency
// appears after its dependent here.
func TestOrderRespectsTheChain(t *testing.T) {
	run := ordered()
	if len(run) != len(writers) {
		t.Fatalf("ordered() returns %d writers and there are %d", len(run), len(writers))
	}
	at := map[string]int{}
	for i, w := range run {
		at[w.Name] = i
	}
	for _, w := range writers {
		for _, need := range w.Needs {
			dep, ok := producer(need)
			if !ok {
				continue
			}
			if at[dep.Name] > at[w.Name] {
				t.Errorf("%s runs at %d and %s, which it needs for %s, runs at %d",
					w.Name, at[w.Name], dep.Name, need, at[dep.Name])
			}
		}
	}
}

// Choosing a writer chooses what its output is about, and says so. The
// alternative is what it used to do: write a file whose whole content is a
// claim about a file nobody wrote.
func TestChoosingAWriterPullsInWhatItsOutputIsAbout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		only   []string
		want   []string
		pulled int
	}{
		{"llms needs the sitemap it points at", []string{"llms"}, []string{"llms", "sitemap"}, 1},
		{"robots names the sitemap too", []string{"robots"}, []string{"robots", "sitemap"}, 1},
		{"a writer that needs nothing pulls nothing", []string{"headers"}, []string{"headers"}, 0},
		{"asking for both is not asking twice", []string{"llms", "sitemap"}, []string{"llms", "sitemap"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &picked{only: cli.ToSet(tc.only), skip: map[string]bool{}}
			if err := chain(cli.Call{}, p); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !p.only[want] {
					t.Errorf("%v did not select %s", tc.only, want)
				}
			}
			if len(p.only) != len(tc.want) {
				t.Errorf("selected %v; want exactly %v", p.only, tc.want)
			}
			if len(p.pulled) != tc.pulled {
				t.Errorf("reported %d pulled in (%v); want %d", len(p.pulled), p.pulled, tc.pulled)
			}
		})
	}
}

// Naming a writer in --only and its dependency in --skip is both halves of a
// contradiction. Quietly honouring either one is worse than saying so.
func TestSkippingWhatAChosenWriterNeedsIsRefused(t *testing.T) {
	p := &picked{only: cli.ToSet([]string{"llms"}), skip: cli.ToSet([]string{"sitemap"})}
	err := chain(cli.Call{}, p)
	if err == nil {
		t.Fatal("writing an llms.txt that points at a sitemap --skip excluded was allowed")
	}
	for _, want := range []string{"llms", "sitemap", "--skip"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
}
