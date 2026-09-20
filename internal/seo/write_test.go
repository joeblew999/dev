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

// A head fragment promises a mark: two <link rel="icon"> tags and an og:image
// all pointing at one file. Whether that file exists is the icon writer's
// business, and the chain is what makes the promise true — so choosing head
// has to choose it, the same way choosing llms chooses the sitemap.
func TestChoosingTheHeadChoosesTheIconItPointsAt(t *testing.T) {
	p := &picked{only: cli.ToSet([]string{"head"}), skip: map[string]bool{}}
	if err := chain(cli.Call{}, p); err != nil {
		t.Fatal(err)
	}
	if !p.only["icon"] {
		t.Errorf("--only head selected %v; head's tags point at %s and nothing would write it",
			p.only, iconFile)
	}
}

// The tags whose absence a checker reports. og:image is the one that changed:
// it used to be written only when --image was given, so every site that did
// not pass one shipped an incomplete Open Graph set and previewed as a grey
// box. The icon writer guarantees a file, so there is always an answer.
func TestHeadCarriesTheTagsThatWereReportedMissing(t *testing.T) {
	for _, tc := range []struct {
		name string
		site Site
		want []string
	}{
		{"no --image falls back to the icon this also writes",
			Site{Origin: "https://example.com", URL: "https://example.com/", Title: "T", Desc: "D"},
			[]string{
				`<link rel="icon" href="/icon.png">`,
				`<link rel="apple-touch-icon" href="/icon.png">`,
				`<meta property="og:image" content="https://example.com/icon.png">`,
			}},
		{"--image is a real card and wins",
			Site{Origin: "https://example.com", URL: "https://example.com/", Title: "T", Desc: "D",
				Image: "https://example.com/card.png"},
			[]string{`<meta property="og:image" content="https://example.com/card.png">`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, covered, err := writeHead(tc.site)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("head.html does not carry %s\n%s", want, got)
				}
			}
			if !strings.Contains(covered, "icon") {
				t.Errorf("covered = %q; a reader cannot tell the page now has a mark", covered)
			}
		})
	}
}

// An og:image a scraper cannot fetch is worse than none: a broken image where
// a preview would be. With no origin there is no absolute URL to build, and a
// relative one is not resolved.
func TestNoOriginMeansNoOpenGraphImage(t *testing.T) {
	got, _, err := writeHead(Site{URL: "/page", Title: "T", Desc: "D"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "og:image") {
		t.Errorf("head.html names an og:image with no host to fetch it from:\n%s", got)
	}
}

// The icon has to be a PNG a browser and a social scraper will both take, and
// big enough that a link preview keeps it — Facebook and LinkedIn drop one
// under 200×200 and say nothing. Its own validator is what says so, and the
// bytes it validates are the bytes the writer produced.
func TestIconIsAPngBigEnoughToSurviveAPreview(t *testing.T) {
	content, covered, err := writeIcon(Site{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	found, read := validateIcon(iconFile, content, "https://example.com")
	for _, f := range found {
		t.Errorf("the icon just written does not validate: %s %s — %s", f.Severity, f.ID, f.Message)
	}
	for _, want := range []string{covered, read} {
		if !strings.Contains(want, "512×512") {
			t.Errorf("%q does not say what was drawn", want)
		}
	}
	// A file that is not a PNG at all is the failure a browser shows as a
	// broken image and no checker here would otherwise see.
	if found, _ := validateIcon(iconFile, "<svg/>", ""); len(found) == 0 {
		t.Error("an SVG passed as icon.png: no scraper renders one, and nothing said so")
	}
}

// Two sites get two marks, and one site gets the same mark twice. The second
// half is what keeps a rebuild that changed nothing out of the diff, and the
// first is the only reason to draw one at all.
func TestTheMarkIsTheSiteAndOnlyTheSite(t *testing.T) {
	one, _, err := writeIcon(Site{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	again, _, _ := writeIcon(Site{Origin: "https://example.com"})
	other, _, _ := writeIcon(Site{Origin: "https://elsewhere.org"})
	if one != again {
		t.Error("two runs on one origin drew different marks; every rebuild would be a change")
	}
	if one == other {
		t.Error("two origins drew the same mark; the point of it is telling them apart")
	}
}

// Not every header belongs everywhere. A charset says what an HTML page is,
// and saying it on sitemap.xml would be a header claiming a file is something
// it is not — which is the exact fault nosniff exists to stop mattering.
func TestHeadersSayTheCharsetOnPagesAndNowhereElse(t *testing.T) {
	content, covered, err := writeHeaders(Site{CSP: "default-src 'self'"})
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range append([]string{everywhere}, htmlPaths...) {
		if !strings.Contains(content, "\n"+block+"\n") {
			t.Errorf("no %q block:\n%s", block, content)
		}
	}
	// The two that a live site reported and nothing could write.
	for _, want := range []string{"Cache-Control: public, max-age=60", "Content-Type: " + htmlUTF8} {
		if !strings.Contains(content, want) {
			t.Errorf("_headers does not set %q:\n%s", want, content)
		}
	}
	// The charset belongs to the page blocks. If it had reached /* it would
	// be on the sitemap and the icon too, which is the bug this names.
	//
	// Matched as a whole line, because the first version of this looked for
	// "Content-Type" anywhere and found it inside X-Content-Type-Options —
	// a test that failed on a file that was right.
	everywhereBlock, _, _ := strings.Cut(content[strings.Index(content, everywhere+"\n"):], "\n/")
	if strings.Contains(everywhereBlock, "\n  Content-Type:") {
		t.Errorf("the charset reached %s, so every file claims to be HTML:\n%s", everywhere, everywhereBlock)
	}
	if !strings.Contains(covered, "path") {
		t.Errorf("covered = %q; it does not say the file is now more than one block", covered)
	}
}
