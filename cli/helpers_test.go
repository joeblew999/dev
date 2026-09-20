// The generic helpers are the part of cli every other package reaches for, so
// what they promise is held here rather than inferred from the one call site
// that happens to exercise it. Each test pins the promise its doc comment
// makes — order, what is dropped, what is left alone.
package cli

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSortedFamily(t *testing.T) {
	// Sorted sorts in place and hands the same slice back, which is what lets
	// a builder end with `return Sorted(diff)`.
	in := []string{"c", "a", "b"}
	if got := Sorted(in); !slices.Equal(got, []string{"a", "b", "c"}) || &got[0] != &in[0] {
		t.Errorf("Sorted(%q) = %q; want a,b,c in place", in, got)
	}
	if got := SortedDesc([]string{"a", "c", "b"}); !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Errorf("SortedDesc = %q; want c,b,a", got)
	}
	if got := SortedKeys(map[string]int{"b": 2, "a": 1}); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("SortedKeys = %q; want a,b", got)
	}
	// SortedBy is the same shape for items that carry no order of their own.
	type file struct{ name string }
	by := SortedBy([]file{{"b"}, {"a"}}, func(x, y file) int { return strings.Compare(x.name, y.name) })
	if by[0].name != "a" || by[1].name != "b" {
		t.Errorf("SortedBy = %v; want a,b", by)
	}
}

func TestMapCollectFilter(t *testing.T) {
	if got := Map([]int{1, 2, 3}, func(n int) int { return n * 2 }); !slices.Equal(got, []int{2, 4, 6}) {
		t.Errorf("Map = %v; want 2,4,6", got)
	}
	// Collect is Map and Filter in one pass: a rewrite that fails is dropped,
	// not zero-valued.
	got := Collect([]string{"a=1", "bad", "b=2"}, func(s string) (string, bool) {
		k, _, ok := strings.Cut(s, "=")
		return k, ok
	})
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("Collect = %q; want a,b", got)
	}
	if got := Filter([]int{1, 2, 3, 4}, func(n int) bool { return n%2 == 0 }); !slices.Equal(got, []int{2, 4}) {
		t.Errorf("Filter = %v; want 2,4", got)
	}
}

func TestSetHelpers(t *testing.T) {
	if set := ToSet([]string{"a", "b", "a"}); len(set) != 2 || !set["a"] || !set["b"] {
		t.Errorf("ToSet = %v; want a,b", set)
	}
	// Unique keeps the first of each, in the order they arrived: the reason a
	// repeated name in an Order list degrades to one entry rather than sorting
	// the list out from under the author.
	if got := Unique([]string{"b", "a", "b", "c"}); !slices.Equal(got, []string{"b", "a", "c"}) {
		t.Errorf("Unique = %q; want b,a,c", got)
	}
	if got := Without([]string{"a", "lock", "b", "lock"}, "lock"); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("Without = %q; want a,b", got)
	}
	// Pick narrows a map to the keys asked for, and a key the map does not
	// have stays absent rather than arriving as a zero value — which is what
	// makes it safe to compose with a diff.
	got := Pick(map[string]int{"a": 1, "b": 2, "c": 3}, []string{"a", "c", "missing"})
	if !reflect.DeepEqual(got, map[string]int{"a": 1, "c": 3}) {
		t.Errorf("Pick = %v; want a,c only", got)
	}
}

func TestDiffSetsKeepsEachSidesOrder(t *testing.T) {
	gone, arrived := DiffSets([]string{"c", "a", "b"}, []string{"b", "z"})
	if !slices.Equal(gone, []string{"c", "a"}) {
		t.Errorf("gone = %q; want c,a in allowed's order", gone)
	}
	if !slices.Equal(arrived, []string{"z"}) {
		t.Errorf("arrived = %q; want z", arrived)
	}
}

func TestDiffMaps(t *testing.T) {
	have := map[string]string{"same": "1", "changed": "2", "extra": "3", "skipme": "x"}
	want := map[string]string{"same": "1", "changed": "9", "gone": "4", "skipme": "y"}
	equal := func(a, b string) bool { return a == b }
	got := DiffMaps(have, want, equal, "skipme")
	if !slices.Equal(got, []string{"changed: changed", "missing: gone", "unexpected: extra"}) {
		t.Errorf("DiffMaps = %q; want changed, missing, unexpected, sorted, with skipme ignored", got)
	}
	if got := DiffMaps(have, have, equal); len(got) != 0 {
		t.Errorf("DiffMaps against itself = %q; want none", got)
	}
}

// DiffKeys is DiffMaps over both sides narrowed to the keys it owns, so what
// matters is that keys outside the list are invisible — including a key
// neither side has, which is not a difference.
func TestDiffKeysSeesOnlyTheKeysItOwns(t *testing.T) {
	owned := []string{"a", "b", "absent"}
	have := map[string]string{"a": "1", "b": "2", "theirs": "x"}
	want := map[string]string{"a": "1", "b": "9", "theirs": "y"}
	equal := func(x, y string) bool { return x == y }
	if got := DiffKeys(owned, have, want, equal); !slices.Equal(got, []string{"changed: b"}) {
		t.Errorf("DiffKeys = %q; want only changed: b", got)
	}
	// One side holding a key the other does not is the missing/unexpected pair.
	got := DiffKeys(owned, map[string]string{"a": "1"}, map[string]string{"b": "2"}, equal)
	if !slices.Equal(got, []string{"missing: b", "unexpected: a"}) {
		t.Errorf("DiffKeys = %q; want missing: b, unexpected: a", got)
	}
}

// Reports count things constantly, and "1 pages" is the tell that one of
// them was written by hand.
func TestPlural(t *testing.T) {
	for _, tc := range []struct {
		n    int
		word string
		want string
	}{
		{0, "page", "0 pages"},
		{1, "page", "1 page"},
		{2, "page", "2 pages"},
		{1, "broken link", "1 broken link"},
		{7, "broken link", "7 broken links"},
		// A consonant before a final y: "2 directorys" reached a real report.
		{2, "directory", "2 directories"},
		{1, "directory", "1 directory"},
		// A vowel before it does not: these stay regular.
		{3, "key", "3 keys"},
		// Already sibilant, so -es rather than a second s.
		{2, "class", "2 classes"},
		{2, "fix", "2 fixes"},
		{2, "patch", "2 patches"},
		{0, "", "0 "},
	} {
		if got := Plural(tc.n, tc.word); got != tc.want {
			t.Errorf("Plural(%d, %q) = %q; want %q", tc.n, tc.word, got, tc.want)
		}
	}
}

// Or is the default a caller falls back to when a value is blank.
func TestOr(t *testing.T) {
	if got := Or("", "fallback"); got != "fallback" {
		t.Errorf("Or(\"\", fallback) = %q", got)
	}
	if got := Or("given", "fallback"); got != "given" {
		t.Errorf("Or(given, fallback) = %q", got)
	}
}

// One situation, one voice. Six places answered "that is not one of the names
// there are" in three different ways, and a reader meeting it in two commands
// should not have to work out that they are the same.
func TestUnknownSaysTheSameThingEverywhere(t *testing.T) {
	have := []string{"kitsune", "scoutly", "muffet"}
	// A typo has one answer, and reading a list to find it is work nobody
	// needs to do.
	if got := Unknown("checker", "kitsuen", have).Error(); !strings.Contains(got, `did you mean "kitsune"`) {
		t.Errorf("a near miss was not offered the name it meant: %s", got)
	}
	// A name resembling nothing usually means the reader does not know what
	// exists, so the list is the answer — as a sentence, sorted.
	got := Unknown("preset", "quantum", have).Error()
	if !strings.Contains(got, "kitsune, muffet and scoutly") {
		t.Errorf("a wild miss was not told what exists: %s", got)
	}
	if !strings.Contains(got, `no preset named "quantum"`) {
		t.Errorf("the error does not name what kind of thing was missing: %s", got)
	}
	// Nothing to offer is its own sentence: listing none reads as a bug.
	if got := Unknown("preset", "x", nil).Error(); !strings.Contains(got, "there are none") {
		t.Errorf("an empty registry said: %s", got)
	}
}

// English is how a list reaches a sentence. Two packages had written it, one
// of them twice, during the work that was meant to remove duplication.
func TestEnglish(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b and c"},
		{[]string{"a", "b", "c", "d"}, "a, b, c and d"},
	} {
		if got := English(tc.in); got != tc.want {
			t.Errorf("English(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// Nearest has to catch the typo people actually make. Two adjacent letters
// swapped is the commonest one, and plain Levenshtein counts it as two edits
// while the bound admits one for a short name — so it was never caught. It
// must still refuse a name that merely rhymes, which is why the bound did not
// move.
func TestNearestCatchesASwapButNotAStranger(t *testing.T) {
	checkers := []string{"kitsune", "scoutly", "muffet", "scry"}
	for typo, want := range map[string]string{
		"kitsuen":  "kitsune", // two letters swapped
		"kitsune":  "kitsune", // exact
		"kitsun":   "kitsune", // one missing
		"kitsunee": "kitsune", // one extra
		"muffte":   "muffet",  // swapped, shorter name
	} {
		if got := Nearest(typo, checkers); got != want {
			t.Errorf("Nearest(%q) = %q; want %q", typo, got, want)
		}
	}
	for _, stranger := range []string{"quantum", "everything", "robots"} {
		if got := Nearest(stranger, checkers); got != "" {
			t.Errorf("Nearest(%q) offered %q; a name that resembles nothing gets no guess", stranger, got)
		}
	}
	// The bound that made this necessary: a severity list, where "everything"
	// was once offered "warning".
	if got := Nearest("everything", []string{SevError, SevWarning, SevInfo}); got != "" {
		t.Errorf("Nearest offered %q for a word that is not a severity", got)
	}
}

// Set is whether a flag was actually given, which Given cannot answer: Given
// is Value == "true", so it speaks for bools alone, and a string flag's only
// other signal is a value differing from its default — which cannot tell
// `--local ""` from no --local at all.
func TestSetKnowsWhatWasActuallyGiven(t *testing.T) {
	newFlags := func() *flag.FlagSet {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.String("local", "", "")
		fs.String("env", "dev", "")
		fs.Var(new(Bool), "refresh", "")
		return fs
	}
	fs := newFlags()
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"local", "env", "refresh"} {
		if Set(fs, name) {
			t.Errorf("Set(%q) is true with nothing given", name)
		}
	}
	fs = newFlags()
	if err := fs.Parse([]string{"--local", "", "--env", "dev", "--refresh=false"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"local", "env", "refresh"} {
		if !Set(fs, name) {
			t.Errorf("Set(%q) is false though it was given", name)
		}
	}
	// Each of those is invisible to Given and to a default comparison: an
	// empty --local, an --env equal to its default, and a bool set to false.
	if Given(fs, "refresh") {
		t.Error("Given says --refresh=false is set; that is what Set is for")
	}
	if Set(fs, "nosuchflag") {
		t.Error("Set is true for a flag that is not registered")
	}
}

// A list of alternatives is not a list of things. "has no wrangler.toml and
// fly.toml" reads as needing both, which is the opposite of what it means.
func TestEitherOr(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a or b"},
		{[]string{"a", "b", "c"}, "a, b or c"},
	} {
		if got := EitherOr(tc.in); got != tc.want {
			t.Errorf("EitherOr(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
	// And English still says and, because a set of destinations written to is
	// a list of things.
	if got := English([]string{"a", "b"}); got != "a and b" {
		t.Errorf("English = %q", got)
	}
}

// A repo adopts this command by pinning one line, and then finds out what
// else it needs one failure at a time — wrangler when it first deploys a
// Worker, node because wrangler runs on it, flyctl at the first Fly deploy,
// fnox the moment anything touches a secret. Each is a stop, a search and a
// guess at a version.
//
// One declaration answers both: the list up front, and the line a missing
// binary names. They cannot disagree because there is only one of them.
func TestEveryToolSaysWhatItIsForAndHowToGetIt(t *testing.T) {
	all := Needs()
	if len(all) == 0 {
		t.Fatal("no tools declared, so `tools` answers nothing")
	}
	for _, n := range all {
		if n.For == "" {
			t.Errorf("%s does not say what it is for, so nobody can tell whether they need it", n.Bin)
		}
		// Either mise can install it and there is a line, or it cannot and
		// there is a sentence. Neither is optional: a tool with no line and
		// no reason is one a reader can do nothing about.
		if n.Pin == "" && n.Why == "" {
			t.Errorf("%s has no pin and no explanation", n.Bin)
		}
		if n.Line() == "" {
			t.Errorf("%s offers nothing when it is missing", n.Bin)
		}
	}
	// The three mise cannot install are named as such, so an error does not
	// offer a mise line for git.
	for _, bin := range []string{"git", "ps", "lsof", "claude"} {
		if Installable(bin) {
			t.Errorf("%s is offered as a mise install and is not one", bin)
		}
		if PinFor(bin) == "" {
			t.Errorf("%s says nothing when it is missing", bin)
		}
	}
	// And the ones it can.
	for _, bin := range []string{"wrangler", "flyctl", "fnox", "cloudflared", "go"} {
		if !Installable(bin) {
			t.Errorf("%s has no mise line, and mise installs it", bin)
		}
		if !strings.Contains(PinFor(bin), "=") {
			t.Errorf("%s's line is not a [tools] entry: %q", bin, PinFor(bin))
		}
	}
	// Every binary the tree actually runs is declared. This is the half that
	// rots: a new tool gets a Cmd and nobody remembers the registry, and the
	// first person to hear about it is whoever adopts the command next.
	for _, bin := range []string{"go", "npm", "node", "packslip", "fnox", "gh",
		"goreleaser", "tinygo", "claude", "flyctl", "git", "lsof", "ps", "wrangler"} {
		if PinFor(bin) == "" {
			t.Errorf("%s is run by this tree and is not declared in needs", bin)
		}
	}
}

// The registry's own rules run from main_test.go, where every package that
// registers a tool is linked and the whole set is visible — here only cli's
// own are, which is how a checker's pin went unchecked for as long as it did.
//
// What is left is the two cases the rule was written for, named rather than
// derived, so a refactor that loses the distinction is caught by name.
func TestTheMiseKeyMatchesThePin(t *testing.T) {
	// The two this was written for, so a refactor that loses the distinction
	// is caught by name rather than by the rule alone.
	for bin, key := range map[string]string{"tofu": "opentofu", "npm": "node"} {
		var found bool
		for _, n := range Needs() {
			if n.Bin == bin {
				found = true
				if n.MiseKey() != key {
					t.Errorf("%s should be asked of mise as %q, not %q", bin, key, n.MiseKey())
				}
			}
		}
		if !found {
			t.Errorf("%s is no longer declared", bin)
		}
	}
	// A backend path is quoted in TOML and bare in a spec, which is the one
	// place the two spellings genuinely differ.
	backend := Need{Bin: "hk", Pin: "packslip:github.com/jdx/hk@2.0.1"}
	if got := backend.Line(); got != `"packslip:github.com/jdx/hk" = "2.0.1"` {
		t.Errorf("a backend path renders as %q", got)
	}
	if got := backend.Spec(); got != "packslip:github.com/jdx/hk@2.0.1" {
		t.Errorf("a backend spec is %q", got)
	}
}

// The config this writes to is this directory's own, and nothing else.
//
// Asking mise which config it would write to answers with the whole
// precedence chain, and the first line of that is the machine's global one —
// so a --fresh in an empty scratch directory read ~/.config/mise/config.toml
// as "the config here" and set about replacing a machine's own settings. It
// failed on a path quirk rather than on judgement.
func TestTheConfigIsThisDirectorysOwn(t *testing.T) {
	t.Chdir(t.TempDir())
	if got := configHere(); got != "" {
		t.Fatalf("an empty directory reported %q as its config; nothing above it is this command's to touch", got)
	}
	if err := os.WriteFile("mise.toml", []byte("[tools]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := configHere(); got != "mise.toml" {
		t.Errorf("configHere() = %q; want the file in this directory", got)
	}
	// Whatever it returns is a bare name, never a path leading elsewhere.
	if got := configHere(); filepath.IsAbs(got) || strings.Contains(got, "..") {
		t.Errorf("configHere() = %q; it must not name anything outside this directory", got)
	}
}

// Two tools can want one pin — node ships npm — and mise given the same spec
// twice once wrote the same key twice into a [tools] table, which is not
// valid TOML. The specs handed over are unique.
func TestOnePinIsAskedForOnce(t *testing.T) {
	var shared []Need
	for _, n := range Needs() {
		if n.Bin == "node" || n.Bin == "npm" {
			shared = append(shared, n)
		}
	}
	if len(shared) != 2 {
		t.Fatalf("expected node and npm to be declared; got %d", len(shared))
	}
	if shared[0].Spec() != shared[1].Spec() {
		t.Skip("node and npm no longer share a pin")
	}
	specs := Unique(Collect(shared, func(n Need) (string, bool) { return n.Spec(), n.Pin != "" }))
	if len(specs) != 1 {
		t.Errorf("two tools sharing one pin produced %v; mise must be asked once", specs)
	}
}

// Four reports in this tree built a step by hand: start a clock, call the
// thing, stop the clock, fill a Step, record it. The fifth line varied only
// by being forgotten — findings attributed to the part in one place and not
// in another, so a report could name a problem without naming what noticed
// it.
func TestMeasureTimesAndAttributes(t *testing.T) {
	got := Measure("probe", "what it is for", func() ([]Finding, string, error) {
		return []Finding{{ID: "a"}, {ID: "b", Tool: "its own"}}, "two things", nil
	})
	if got.Step.Name != "probe" || got.Step.Provides != "what it is for" {
		t.Errorf("step = %+v", got.Step)
	}
	if got.Step.Covered != "two things" || got.Step.Findings != 2 {
		t.Errorf("step did not carry what the part said: %+v", got.Step)
	}
	if got.Step.Took == "" {
		t.Error("the step was not timed")
	}
	// Attribution is filled in, and what a part named itself is kept: a
	// checker that knows better than the step it belongs to should say so.
	if got.Findings[0].Tool != "probe" {
		t.Errorf("a finding was not attributed: %+v", got.Findings[0])
	}
	if got.Findings[1].Tool != "its own" {
		t.Errorf("an attribution the part made was overwritten: %+v", got.Findings[1])
	}
}

// A part that could not run is a step with a reason, not a failed report —
// and it carries what it would have provided, so a reader can decide whether
// to care.
func TestMeasureRecordsWhatDidNotRun(t *testing.T) {
	got := Measure("probe", "what it is for", func() ([]Finding, string, error) {
		return nil, "", errors.New("no credentials")
	})
	if got.Step.Status != StatusSkipped {
		t.Errorf("status = %q; a part that could not run did not run", got.Step.Status)
	}
	if got.Step.Note != "no credentials" {
		t.Errorf("note = %q; the reason is the point", got.Step.Note)
	}
	if got.Step.Provides == "" {
		t.Error("a skipped step dropped what it would have given")
	}
	if len(got.Findings) != 0 {
		t.Error("a part that could not run reported findings")
	}
}

// Recorded in the order given however they were scheduled, so a report reads
// the same whether it ran one at a time or all at once.
func TestGatherKeepsTheOrderItWasGiven(t *testing.T) {
	r := NewReport("t", "x")
	parts := []string{"first", "second", "third", "fourth"}
	Gather(r, len(parts), parts,
		func(s string) string { return s },
		func(s string) Measured {
			return Measure(s, "p", func() ([]Finding, string, error) { return nil, s, nil })
		})
	for i, s := range r.Steps {
		if s.Name != parts[i] {
			t.Fatalf("step %d is %q; want %q — order must not depend on scheduling", i, s.Name, parts[i])
		}
	}
}

// A part that panics keeps its name, because the thing that would have named
// it is the thing that did not finish. Without a namer the report said the
// whole value, which for a struct is unreadable.
func TestAPanickingPartIsNamedAndTheRestSurvive(t *testing.T) {
	r := NewReport("t", "x")
	type checker struct{ Name, Long string }
	parts := []checker{{"good", "aaa"}, {"bad", "bbb"}, {"also good", "ccc"}}
	Gather(r, len(parts), parts,
		func(c checker) string { return c.Name },
		func(c checker) Measured {
			if c.Name == "bad" {
				panic("upstream changed its mind")
			}
			return Measure(c.Name, "p", func() ([]Finding, string, error) { return nil, "ok", nil })
		})
	if len(r.Steps) != 3 {
		t.Fatalf("recorded %d steps; one panicking part must not lose the others", len(r.Steps))
	}
	if r.Steps[1].Name != "bad" {
		t.Errorf("the panicking step is named %q", r.Steps[1].Name)
	}
	if r.Steps[1].Status != StatusSkipped || !strings.Contains(r.Steps[1].Note, "upstream changed its mind") {
		t.Errorf("the panic was not recorded against it: %+v", r.Steps[1])
	}
	for _, i := range []int{0, 2} {
		if r.Steps[i].Status == StatusSkipped {
			t.Errorf("step %d did not survive its neighbour's panic", i)
		}
	}
}

// A column that fits what is in it. A fixed width is right until a report is
// about domains rather than three checks with short names, and then every
// line sits a character out from its neighbour.
func TestWidest(t *testing.T) {
	type row struct{ name string }
	rows := []row{{"dns"}, {"arrangement"}, {"ssl"}}
	if got := Widest(rows, func(r row) string { return r.name }); got != len("arrangement") {
		t.Errorf("Widest = %d; want %d", got, len("arrangement"))
	}
	// An empty list is zero, which every format verb accepts as "no padding"
	// rather than as an error.
	if got := Widest(nil, func(r row) string { return r.name }); got != 0 {
		t.Errorf("Widest(nil) = %d; want 0", got)
	}
}
