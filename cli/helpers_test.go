// The generic helpers are the part of cli every other package reaches for, so
// what they promise is held here rather than inferred from the one call site
// that happens to exercise it. Each test pins the promise its doc comment
// makes — order, what is dropped, what is left alone.
package cli

import (
	"flag"
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
