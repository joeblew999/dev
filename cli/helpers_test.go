// The generic helpers are the part of cli every other package reaches for, so
// what they promise is held here rather than inferred from the one call site
// that happens to exercise it. Each test pins the promise its doc comment
// makes — order, what is dropped, what is left alone.
package cli

import (
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
