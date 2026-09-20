// Package cli is the verb system every command on the stack shares: one
// shape for a command (command.go's Command), and the helpers its verbs use
// to read flags and the directory they act on (this file). A command declares
// its verbs and its prose; Main runs it and renders its manual from the same
// table, so the manual cannot drift from the binary.
package cli

import (
	"bufio"
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Runner is every dev command: the verb it was called as, the arguments after
// it, and where to write. A package that answers to several verbs (build,
// wasm, check, run, workerd are all stage's) switches on verb.
type Runner func(Call) error

// Call is one invocation of a verb, parsed. A verb declares its Args and its
// Flags, so cli can read them once and hand the result over — rather than
// every verb opening with the same four lines to build a FlagSet, parse a
// directory out of the arguments and check the error.
//
// That preamble was seventeen copies of one idea. A verb's body is now the
// work it does.
type Call struct {
	Verb  string        // the verb as it was typed, with any subcommand: "secrets push"
	Dir   string        // the directory it acts on, when its Args begin with DIR
	Args  []string      // what is left: positionals, then anything after a bare --
	Flags *flag.FlagSet // parsed, so Value and Given read from it
	// Command is the binary's name, for the environment variables a flag
	// falls back to: `dev seo write --title` reads DEV_TITLE when no --title
	// was given. Empty in a Call built by hand, which then reads no
	// environment at all — a test gets what it passed and nothing else.
	Command string

	Stdin          io.Reader
	Stdout, Stderr io.Writer

	// Started is when cli began running this verb. A verb that reports in
	// JSON puts its own elapsed time in the report from it, so what a person
	// reads on stderr and what a machine reads in the file are the same run.
	Started time.Time
}

// Elapsed is how long this verb has been running. Zero when nothing set
// Started, so a Call built by hand in a test reports no time rather than the
// centuries since the zero instant.
func (c Call) Elapsed() time.Duration {
	if c.Started.IsZero() {
		return 0
	}
	return time.Since(c.Started)
}

// Took is a duration as a person reads one at the end of a run: milliseconds
// while that is the interesting digit, then seconds, then whole seconds once
// a run is long enough that nobody is counting them.
func Took(d time.Duration) string {
	switch {
	case d < time.Second:
		return d.Round(time.Millisecond).String()
	case d < time.Minute:
		return d.Round(10 * time.Millisecond).String()
	default:
		return d.Round(time.Second).String()
	}
}

// Value is a parsed flag's value by name.
func (c Call) Value(name string) string {
	if v := Value(c.Flags, name); v != "" {
		return v
	}
	return fromEnv(c.Command, name)
}

// EnvName is what a flag is called in the environment: the command, then the
// flag, upper-cased with hyphens as underscores. `dev seo write --title` is
// DEV_TITLE.
//
// Named by the flag rather than by the verb because a flag name on this stack
// already means one thing wherever it appears — --url is the site, --title is
// what Search shows — which is why pageFlags exists at all. A second spelling
// per verb would be DEV_SEO_WRITE_TITLE and DEV_SEO_CHECK_TITLE for one fact.
func EnvName(command, flag string) string {
	return strings.ToUpper(command + "_" + strings.ReplaceAll(flag, "-", "_"))
}

// fromEnv is a flag's value when nobody passed it.
//
// mise is how a repo on this stack says what it wants, and a task that spells
// out eight flags is a repo saying it in the wrong place: the facts end up in
// two tasks, drift between them, and the task grows until nobody reads it.
// Under [env] in mise.toml they are declared once and every task that calls
// the command gets them — including the ones the tests run, which is what
// stops a test asserting against a different site than the one deployed.
//
// The flag still wins when it is given, so a task can say something different
// without the declaration moving.
func fromEnv(command, flag string) string {
	if command == "" {
		return ""
	}
	return os.Getenv(EnvName(command, flag))
}

// Given reports whether a bool flag is set.
func (c Call) Given(name string) bool { return Given(c.Flags, name) }

// Usagef is an argument error, naming this verb.
func (c Call) Usagef(format string, a ...any) error {
	return Usagef("%s: %s", c.Verb, fmt.Sprintf(format, a...))
}

// UsageError means the arguments were wrong; main prints the package's usage
// after it.
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

// Usagef makes a UsageError — unless what it is reporting is a request for
// help, which every verb passes here by writing `Usagef("%s: %v", verb, err)`
// around whatever parsing returned.
//
// It has to be caught here rather than at each call site. Asking every verb in
// every repo to remember one line is asking it to be forgotten, and it was:
// the first fix made propagation the caller's job, and a command in another
// repo still answered --help with "error: flag: help requested".
func Usagef(format string, a ...any) error {
	for _, arg := range a {
		if err, ok := arg.(error); ok && errors.Is(err, flag.ErrHelp) {
			return ErrHelp
		}
	}
	return &UsageError{fmt.Sprintf(format, a...)}
}

// Flags is a flag set that reports to stderr and never exits.
func Flags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// Bool is a flag.Value for booleans that also takes "" as false, so a task
// may pass `--flag=$var` with the variable unset.
type Bool bool

func (b *Bool) String() string { return fmt.Sprint(bool(*b)) }

// IsBoolFlag lets a bare `--flag` mean true.
func (b *Bool) IsBoolFlag() bool { return true }

// Set accepts the usual spellings of true and false, and "" as false.
func (b *Bool) Set(s string) error {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "0", "false", "no", "off":
		*b = false
	case "1", "true", "yes", "on":
		*b = true
	default:
		return fmt.Errorf("want true or false, got %q", s)
	}
	return nil
}

// Stdin is where a verb that asks a question reads the answer. A variable so
// a test can replace it.
var Stdin io.Reader = os.Stdin

// ErrHelp is what a verb returns when it was asked for help rather than run.
// The flag package has already printed the flags and what each one means, so
// the command prints the verb's own usage after it and exits 0: asking what a
// verb takes is not an error, and reporting it as one is how the flags'
// descriptions stayed invisible.
var ErrHelp = flag.ErrHelp

// HelpRequested reports whether args ask what a verb takes, rather than ask
// it to run. Everything after a bare "--" belongs to the program being run, so
// its --help is not ours.
//
// A verb that dispatches on a subcommand, or that wants DIR before anything
// else, has to ask this before it enforces either: otherwise the question is
// answered with "the directory comes first", which is true and useless.
func HelpRequested(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// DirAnd parses "DIR [flags and positionals in any order]": the directory a
// command acts on comes first; positional is how many positionals follow, or
// -1 for any number. Flags may come anywhere, because mise appends what the
// developer typed after a task name to the command; everything after a bare
// "--" is positional, verbatim.
func DirAnd(fs *flag.FlagSet, args []string, positional int) (dir string, rest []string, err error) {
	// Before the directory is required, because asking what the verb takes is
	// how someone finds out that it wants one.
	if HelpRequested(args) {
		fs.Usage()
		return "", nil, ErrHelp
	}
	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		return "", nil, Usagef("%s: the directory comes first", fs.Name())
	}
	rest, err = ParseInterleaved(fs, args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return "", nil, ErrHelp
	}
	if err != nil {
		return "", nil, Usagef("%s: %v", fs.Name(), err)
	}
	if positional >= 0 && len(rest) != positional {
		return "", nil, Usagef("%s: wrong arguments", fs.Name())
	}
	return args[0], rest, nil
}

// ParseInterleaved parses flags wherever they appear and returns the
// positionals, with everything after a bare "--" appended verbatim.
func ParseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals, tail []string
	for i, a := range args {
		if a == "--" {
			args, tail = args[:i], args[i+1:]
			break
		}
	}
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) > 0 {
			positionals = append(positionals, args[0])
			args = args[1:]
		}
	}
	return append(positionals, tail...), nil
}

// Confirm asks on out and reads one line from stdin: y or yes means yes.
// Anything else, including no terminal to answer from, means no.
func Confirm(stdin io.Reader, out io.Writer, prompt string) bool {
	fmt.Fprint(out, prompt)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(out)
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// Value is a parsed flag's value by name, for a verb that registers its flags
// through a func rather than holding each pointer. The registration is one
// place so that cli can render a signature from it; reading back by name is
// what that costs, and it costs nothing at the call site.
//
// An unregistered name gives "", because a verb asking for a flag it never
// registered is a bug in the verb, not a condition to handle at runtime.
func Value(fs *flag.FlagSet, name string) string {
	if f := fs.Lookup(name); f != nil {
		return f.Value.String()
	}
	return ""
}

// Given reports whether a bool flag registered by name is set.
func Given(fs *flag.FlagSet, name string) bool {
	return Value(fs, name) == "true"
}

// Set reports whether a flag was actually given, whatever its type.
//
// Given cannot answer this: it is Value == "true", so it speaks for bools
// alone, and for a string flag the only signal is a value differing from the
// default — which cannot tell "--local ”" from not passing --local. Visit
// walks the flags that were set and nothing else, so this is exact.
func Set(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// Set reports whether this call was given the named flag.
func (c Call) Set(name string) bool { return Set(c.Flags, name) }

// ValueAs parses a named flag's value with parse, for flags that are not
// strings: durations, ints, and the like. It is a generic method, which Go
// 1.27 allows: before, this had to be a generic function taking the Call,
// and every call site read as ValueAs(c, ...) rather than c.ValueAs(...).
//
// An unregistered name or a parse failure returns the zero value and the
// parse error, so a verb that wants the zero value on failure ignores the
// error the same way it does with Value plus ParseDuration today.
func (c Call) ValueAs[T any](name string, parse func(string) (T, error)) (T, error) {
	return ValueAs(c.Flags, name, parse)
}

// ValueAs parses a flag's value from its FlagSet with parse. A verb that
// holds only the set calls this; a verb with a Call calls the method.
func ValueAs[T any](fs *flag.FlagSet, name string, parse func(string) (T, error)) (T, error) {
	var zero T
	f := fs.Lookup(name)
	if f == nil {
		return zero, fmt.Errorf("no flag %q", name)
	}
	return parse(f.Value.String())
}

// SortedKeys is every key of m in order. Five call sites wrote
// slices.Sorted(maps.Keys(m)) by hand, which is one idea in five places —
// the thing this stack keeps deleting. The constraint is cmp.Ordered rather
// than any, because sorting needs it and the compiler should say so.
func SortedKeys[M ~map[K]V, K cmp.Ordered, V any](m M) []K {
	return slices.Sorted(maps.Keys(m))
}

// Sorted sorts s in place and returns it, so a diff builder ends with
// `return Sorted(diff)` instead of a Sort line plus a return line. Every
// diff in the tree ended that way, which is one idea in eight places.
func Sorted[S ~[]E, E cmp.Ordered](s S) S {
	slices.Sort(s)
	return s
}

// SortedDesc is every item deepest-first, for removing empty directories
// after their contents: a directory goes after what it holds.
func SortedDesc[S ~[]E, E cmp.Ordered](s S) S {
	slices.Sort(s)
	slices.Reverse(s)
	return s
}

// SortedBy is Sorted for items that are not ordered on their own: the same
// sort-then-return shape, with the comparison the caller's. A struct slice
// ended with slices.SortFunc plus a return in as many places as an ordered
// one ended with slices.Sort, and only the ordered half had a helper.
func SortedBy[S ~[]E, E any](s S, cmp func(a, b E) int) S {
	slices.SortFunc(s, cmp)
	return s
}

// Map rewrites each item, so a titles-from-bindings loop is one call rather
// than a var plus a range.
func Map[T, U any](items []T, f func(T) U) []U {
	out := make([]U, 0, len(items))
	for _, item := range items {
		out = append(out, f(item))
	}
	return out
}

// Collect keeps the rewrites that succeed, so a decoder that hands back
// []any collapses to one call rather than a switch plus a loop.
func Collect[T, U any](items []T, f func(T) (U, bool)) []U {
	var out []U
	for _, item := range items {
		if v, ok := f(item); ok {
			out = append(out, v)
		}
	}
	return out
}

// ToSet is the membership set for items, so a diff stops opening with the
// same four-line loop that builds one map per side.
func ToSet[T comparable](items []T) map[T]bool {
	set := make(map[T]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}

// Unique keeps the first of each item, so an Order list with a repeated
// name degrades to one entry rather than printing a verb twice.
func Unique[T comparable](items []T) []T {
	seen := make(map[T]bool, len(items))
	var out []T
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}

// Filter keeps what keep wants, so collecting one skill's files stops
// opening with the same loop over every file in the set.
func Filter[T any](items []T, keep func(T) bool) []T {
	var out []T
	for _, item := range items {
		if keep(item) {
			out = append(out, item)
		}
	}
	return out
}

// Nearest is the candidate closest to s, or "" when none is close enough. A
// mistyped verb or tool name should name itself rather than print a wall of
// usage: the reader already knows what they meant.
//
// The bound is one edit, and a second for every eight characters typed: a
// typo is a slip of a key or two, however long the word. Half the length was
// too generous — it offered "warning" for "everything", which is not a
// correction but a guess, and a wrong guess is worse than none.
func Nearest(s string, candidates []string) string {
	best, score := "", 2+len(s)/8
	for _, c := range candidates {
		if d := editDistance(s, c); d < score {
			best, score = c, d
		}
	}
	return best
}

// editDistance counts the edits between two names, with two adjacent letters
// swapped counting as one.
//
// Plain Levenshtein counts a swap as two — a delete and an insert — and the
// bound above admits one edit for a short name, so "kitsuen" was never offered
// "kitsune". Transposing two letters is the commonest typo there is, and the
// answer is to count it as the single slip it is rather than to raise the
// bound, which is what let "everything" be offered "warning".
//
// This is optimal string alignment: Levenshtein plus the swap, which needs the
// row from two steps back and so three rows rather than two.
func editDistance(a, b string) int {
	prev2, prev, cur := make([]int, len(b)+1), make([]int, len(b)+1), make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
		}
		copy(prev2, prev)
		copy(prev, cur)
	}
	return prev[len(b)]
}

// Plural is a count and its noun, said the way a person would: "1 page",
// "3 pages". Reports count things constantly, and every place that did this
// by hand either wrote out the if or printed "1 pages".
func Plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + plural(word)
}

// plural is English's common rules, which is as far as a report needs to go:
// the words these messages count are ordinary nouns. A consonant before a
// final y becomes -ies, and a word already ending in a sibilant takes -es.
// Without the first of those a report says "2 directorys", which it did.
func plural(word string) string {
	last := len(word) - 1
	if last < 0 {
		return word
	}
	switch {
	case word[last] == 'y' && last > 0 && !isVowel(word[last-1]):
		return word[:last] + "ies"
	case strings.HasSuffix(word, "s"), strings.HasSuffix(word, "x"),
		strings.HasSuffix(word, "ch"), strings.HasSuffix(word, "sh"):
		return word + "es"
	}
	return word + "s"
}

func isVowel(b byte) bool { return strings.IndexByte("aeiouAEIOU", b) >= 0 }

// English joins names the way a sentence does: "a", "a and b", "a, b and c".
//
// Messages list things constantly — the directories written, the sources a
// preset draws from, the presets that exist — and a list printed with Join
// reads as data where a sentence was meant. Two packages had written this,
// one of them twice, during the work that was meant to remove duplication.
func English(items []string) string { return joined(items, "and") }

// EitherOr is English for a list of alternatives rather than a list of
// things: "wrangler.toml or fly.toml". A directory has no wrangler.toml and
// fly.toml reads as needing both, which is the opposite of what it means.
func EitherOr(items []string) string { return joined(items, "or") }

func joined(items []string, conj string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " " + conj + " " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + " " + conj + " " + items[len(items)-1]
}

// Unknown is what to say when a name is not one of the names there are.
//
// Six places asked this question and answered it in three voices: "did you
// mean x?", "dev ships x and y", "it declares x". They are one situation, and
// a reader meeting it in two commands should not have to work out that they
// are the same.
//
// The nearest name comes first when there is one, because a typo has one
// answer and reading a list to find it is work nobody needs to do. Failing
// that the list is the answer, because a name that resembles nothing usually
// means the reader does not know what exists.
//
// Turning a registry of anything into its names is Map's job, which is where
// the generics in this already are: Unknown takes the names and supplies only
// the sentence.
func Unknown(what, name string, have []string) error {
	switch {
	case Nearest(name, have) != "":
		return fmt.Errorf("no %s named %q; did you mean %q?", what, name, Nearest(name, have))
	case len(have) == 0:
		return fmt.Errorf("no %s named %q, and there are none", what, name)
	default:
		return fmt.Errorf("no %s named %q; there is %s", what, name, English(Sorted(have)))
	}
}

// Lines is a text's lines with the blank tail every file ends with dropped.
//
// Not strings.Lines, which keeps the newline on each line and is an iterator:
// a caller reading a config, a lock file or a tool's output wants the lines
// without them, and half of these callers also want an index. Eleven places
// had written strings.SplitSeq(strings.TrimSpace(s), "\n") or a variant, and
// the variants did not agree about the blank line at the end.
func Lines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// CountBy tallies items by whatever key names them — a severity, a tool, a
// status code. The key func decides the map's type, so one func answers every
// "how many of each" a report asks.
func CountBy[K comparable, T any](items []T, key func(T) K) map[K]int {
	out := map[K]int{}
	for _, item := range items {
		out[key(item)]++
	}
	return out
}

// Without drops every skip from items, so skipping the lock file is one
// call rather than a DeleteFunc line with its own closure.
func Without[T comparable](items []T, skip ...T) []T {
	omit := ToSet(skip)
	return Filter(items, func(item T) bool { return !omit[item] })
}

// DiffSets reports what allowed lost and what seen gained: the set diff
// every check is really asking. Gone comes in allowed's order, arrived in
// seen's, so the answer reads the way the inputs did.
func DiffSets[T comparable](allowed, seen []T) (gone, arrived []T) {
	allow, have := ToSet(allowed), ToSet(seen)
	for _, name := range allowed {
		if !have[name] {
			gone = append(gone, name)
		}
	}
	for _, name := range seen {
		if !allow[name] {
			arrived = append(arrived, name)
		}
	}
	return gone, arrived
}

// DiffMaps describes how have differs from want: missing, changed, then
// unexpected. Equal says when two values match, so one func covers bytes,
// hashes and DeepEqual alike. Keys print with %v, because the diff is text
// and the key type is only ever string here. Names in ignore are skipped on
// both sides, so a lock hash never compares against the lock file itself.
func DiffMaps[M1 ~map[K]V1, M2 ~map[K]V2, K cmp.Ordered, V1, V2 any](have M1, want M2, equal func(h V1, w V2) bool, ignore ...K) []string {
	omit := ToSet(ignore)
	var diff []string
	for k, w := range want {
		if omit[k] {
			continue
		}
		h, ok := have[k]
		switch {
		case !ok:
			diff = append(diff, fmt.Sprintf("missing: %v", k))
		case !equal(h, w):
			diff = append(diff, fmt.Sprintf("changed: %v", k))
		}
	}
	for k := range have {
		if omit[k] {
			continue
		}
		if _, ok := want[k]; !ok {
			diff = append(diff, fmt.Sprintf("unexpected: %v", k))
		}
	}
	return Sorted(diff)
}

// Pick is the sub-map of m over keys, so a check that owns three keys of a
// file every tool writes to compares those and leaves the rest alone. A key
// that is not in m is not in the result, which is what makes Pick composable
// with a diff: absent stays absent rather than becoming a zero value.
func Pick[M ~map[K]V, K comparable, V any](m M, keys []K) M {
	out := make(M, len(keys))
	for _, k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

// DiffKeys describes how have differs from want over exactly keys, so a
// settings check that owns three keys stops opening with its own loop over
// them. Equal is reflect.DeepEqual here and bytes.Equal elsewhere.
//
// It is DiffMaps over both sides narrowed to those keys, rather than a second
// walk with its own switch: missing, changed and unexpected are one
// vocabulary, and two implementations of it drift the first time one of the
// three words is reworded.
func DiffKeys[M ~map[K]V, K cmp.Ordered, V any](keys []K, have, want M, equal func(a, b V) bool) []string {
	return DiffMaps(Pick(have, keys), Pick(want, keys), equal)
}

// Indent prefixes every line with two spaces, for error bodies that list
// what differs. Session had its own copy of this; cli owns it now, so both
// read the same two spaces. It is Indent with a capital, because the
// lowercase indent here is the four-space continuation under a signature —
// a different shape for a different reader.
func Indent(s string) string {
	var b strings.Builder
	for _, line := range Lines(s) {
		b.WriteString("  " + line + "\n")
	}
	return b.String()
}

// Widest is the longest of what show says about each item, for a column that
// fits what is in it. Zero for an empty list, which no format verb minds.
//
// Generic because the shape is the same wherever a table is printed and the
// items never are: steps in a report, tools in a list, verbs in a manual.
func Widest[T any](items []T, show func(T) string) int {
	wide := 0
	for _, item := range items {
		if n := len(show(item)); n > wide {
			wide = n
		}
	}
	return wide
}
