// A report: what a verb that checks things found, what ran to find it, and
// what did not run and why.
//
// This is not SEO's, or any one verb's. A verb that runs several tools and
// merges what they say has the same problems whatever the subject — which
// severity fails the build, what a machine reads, what changed since last
// time, and how to say "this did not run" without it looking like "this found
// nothing". Each of those was about to be solved once per verb.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// The severities, most serious first. A finding carries one; --fail-on names
// the least serious that still fails.
const (
	SevError   = "error"
	SevWarning = "warning"
	SevInfo    = "info"
)

// Severities are the three, most serious first. The order is the rank, the
// list is what a summary counts, and --fail-on takes one of them — three
// facts that were a switch, a second switch and a third list.
var Severities = []string{SevError, SevWarning, SevInfo}

// SevRank orders severities, with anything unknown last so it never satisfies
// a gate by accident.
func SevRank(s string) int {
	if i := slices.Index(Severities, s); i >= 0 {
		return i
	}
	return len(Severities)
}

// Summary is what a run found, in the words the findings above it used.
//
// Every severity that occurred, most serious first, because the line used to
// count errors alone and call them "problems" — so `dev domains` printed four
// warnings and two notes and then said "0 problems" underneath them. A
// summary that contradicts what is directly above it is worse than none.
func (r *Report) Summary() string {
	var parts []string
	for _, sev := range Severities {
		if n := r.BySeverity[sev]; n > 0 {
			parts = append(parts, Plural(n, sev))
		}
	}
	if len(parts) == 0 {
		return "nothing to report"
	}
	return strings.Join(parts, ", ")
}

// What a step did. A step that did not run says so rather than reporting
// nothing found, because those read the same and mean opposite things.
const (
	StatusOK      = "ok"
	StatusSkipped = "skipped"
)

// Finding is one thing worth acting on. Fix is the point: an id and a
// severity say what is wrong and not what to do about it.
type Finding struct {
	Tool     string `json:"tool"`
	Severity string `json:"severity"`
	ID       string `json:"id"`
	Message  string `json:"message"`
	Where    string `json:"where,omitempty"`
	Fix      string `json:"fix,omitempty"`

	// FixedBy names the thing in this same command that resolves the
	// finding, when there is one — so a report is not only a list of what is
	// wrong but a route to fixing it. A verb that both checks and produces
	// should say which of its own outputs closes each gap.
	FixedBy string `json:"fixedBy,omitempty"`
}

// Step is one unit of work: what it was, whether it ran, how long it took,
// and — when it did not run — what it needs and what it would have given.
//
// Requires and Provides are why a report can end with a list of what it did
// not do and how to enable it. A report that quietly omits what it skipped
// reads as complete when it is not.
type Step struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Note     string `json:"note,omitempty"`
	Took     string `json:"took"`
	TookMs   int64  `json:"tookMs"`
	Findings int    `json:"findings"`
	Covered  string `json:"covered,omitempty"` // what it looked at, in the words that apply to it
	Requires string `json:"requires,omitempty"`
	Provides string `json:"provides,omitempty"`
	Cost     string `json:"typicalCost,omitempty"`
	Report   string `json:"report,omitempty"` // where this step's own full output was kept
}

// Report is the shape every checking verb answers in, so a task, an agent or
// a person reads one thing however many verbs produced it.
type Report struct {
	Tool    string    `json:"tool"`
	Target  string    `json:"target"`
	RanAt   time.Time `json:"ranAt"`
	Outcome string    `json:"outcome"`
	Took    string    `json:"took"`
	TookMs  int64     `json:"tookMs"`

	// Fail is what to say when the gate trips, for a verb whose failure has
	// a better sentence than the counts. Empty is the counts.
	Fail string `json:"-"`

	BySeverity map[string]int `json:"bySeverity"`
	ByTool     map[string]int `json:"byTool"`
	Steps      []Step         `json:"steps"`
	Findings   []Finding      `json:"findings"`

	mu sync.Mutex
}

// NewReport starts one.
func NewReport(tool, target string) *Report {
	return &Report{Tool: tool, Target: target, RanAt: time.Now().UTC(),
		Steps: []Step{}, Findings: []Finding{}}
}

// Add records a finding. Safe from several goroutines, because steps run in
// parallel and each of them reports as it goes.
func (r *Report) Add(f Finding) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Findings = append(r.Findings, f)
}

// Ran records a step that ran.
func (r *Report) Ran(s Step) {
	s.Status = Or(s.Status, StatusOK)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Steps = append(r.Steps, s)
}

// NotRun records a step that did not, with why — and with what it would have
// given, so the reader can decide whether to care.
func (r *Report) NotRun(s Step, why string) {
	s.Status, s.Note = StatusSkipped, why
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Steps = append(r.Steps, s)
}

// Done totals the report: the counts, the outcome and the elapsed time. The
// outcome is "pass" unless something at or above failOn was found, which is
// the one place a gate's meaning is decided.
func (r *Report) Done(started time.Time, failOn string) {
	r.BySeverity = CountBy(r.Findings, func(f Finding) string { return f.Severity })
	r.ByTool = CountBy(r.Findings, func(f Finding) string { return f.Tool })
	took := time.Since(started)
	r.Took, r.TookMs = Took(took), took.Milliseconds()
	r.Outcome = "pass"
	if r.Failed(failOn) {
		r.Outcome = "fail"
	}
}

// Finish is the tail every checking verb shares: total the report, answer in
// the shape asked for, keep the run when asked and say what moved since the
// last one, then fail when something must be fixed.
//
// write is the only part that differs — how this verb says it in prose —
// and a verb that answers in JSON never calls it.
func (c Call) Finish(r *Report, started time.Time, write func(*Report)) error {
	r.Sort()
	r.Done(started, c.Value("fail-on"))
	if c.WantsJSON() {
		if err := c.EmitJSON(r); err != nil {
			return err
		}
	} else if write != nil {
		write(r)
	}
	if path, err := c.Record(r); err == nil && path != "" && !c.Given("quiet") {
		fmt.Fprintf(c.Stderr, "recorded: %s\n", path)
		if prev, ok := c.Previous(r, path); ok {
			c.drift(prev, r, path)
		}
	}
	if r.Outcome == "pass" {
		return nil
	}
	if r.Fail != "" {
		return fmt.Errorf("%s", r.Fail)
	}
	return fmt.Errorf("%s: %s, %s", r.Target,
		Plural(r.BySeverity[SevError], "error"), Plural(r.BySeverity[SevWarning], "warning"))
}

// drift says what moved since the last recorded run: the point of keeping a
// history is seeing which way things went, not the total.
func (c Call) drift(prev, cur *Report, path string) {
	fixed, arrived := Drift(prev, cur)
	// "No change" used to end it here, which suppressed the standing counts
	// in the one case they are for: a run that moved nothing is exactly when
	// it matters that something has not moved in seven.
	if len(fixed)+len(arrived) == 0 {
		fmt.Fprintf(c.Stderr, "  no change since %s\n", prev.RanAt.Format(time.RFC3339))
		c.stood(cur, path)
		return
	}
	fmt.Fprintf(c.Stderr, "\n  since %s\n", prev.RanAt.Format(time.RFC3339))
	for _, f := range fixed {
		fmt.Fprintf(c.Stderr, "    FIXED  %-12s %s\n", f.Tool, Or(f.ID, f.Message))
	}
	for _, f := range arrived {
		fmt.Fprintf(c.Stderr, "    NEW    %-12s %s — %s\n", f.Tool, f.ID, f.Message)
	}
	c.stood(cur, path)
}

// stood says what has not moved, and for how long.
//
// A finding standing at two is waiting its turn; one standing at seven has
// had six attempts made at it, and saying so is the difference between a
// loop that learns and one that repeats itself. Only the stubborn ones:
// listing everything unchanged would bury the signal in the noise it is
// meant to cut through.
func (c Call) stood(cur *Report, path string) {
	history := c.History(path)
	unkept := ToSet(Map(Unkept(history, cur), func(f Finding) string { return f.ID + "|" + f.Message }))
	for _, f := range cur.Findings {
		n := Standing(history, f)
		if n <= 2 {
			continue
		}
		// A finding nothing claims is a hard problem. A finding something
		// claims, that has stood anyway, is a different thing entirely: the
		// registry says a writer resolves it, the writer has run, and it is
		// still here. That is a claim the code does not keep, and it is worse
		// than an unclaimed finding because a loop acting on the report will
		// rewrite the same file forever and report progress.
		//
		// Four of these were found by hand in this tree on the day it was
		// written — head claiming thin-content when a head fragment is not a
		// page body, head claiming VIEWPORT while writing no viewport tag,
		// sitemap claiming seo.sitemap.unreferenced when it is robots.txt
		// that does the referencing. Nothing could have told them apart from
		// a hard problem, so nobody looked.
		if unkept[f.ID+"|"+f.Message] {
			fmt.Fprintf(c.Stderr, "    UNFIXED %-11s %s — %s claims this and %s have not closed it\n",
				f.Tool, Or(f.ID, f.Message), f.FixedBy, Plural(n, "run"))
			continue
		}
		fmt.Fprintf(c.Stderr, "    STOOD  %-12s %s — unchanged across %s\n",
			f.Tool, Or(f.ID, f.Message), Plural(n, "run"))
	}
}

// Unkept is every finding something claimed to fix that has survived being
// fixed. A registry that over-claims is worse than one that under-claims: a
// reader told "this file fixes it" writes the file and the fault is still
// there, and a loop told the same thing never stops.
//
// Exported because it is what a test should assert on a project that runs
// this loop — a claim held for three runs is a bug in the declaration, not a
// hard problem, and it can be failed on rather than read.
func Unkept(history []*Report, cur *Report) []Finding {
	return Filter(cur.Findings, func(f Finding) bool {
		return f.FixedBy != "" && Standing(history, f) > 2
	})
}

// Failed reports whether anything was found at or above failOn. An empty
// failOn means errors fail and nothing else does.
func (r *Report) Failed(failOn string) bool {
	want := SevRank(Or(failOn, SevError))
	for _, f := range r.Findings {
		if SevRank(f.Severity) <= want {
			return true
		}
	}
	return false
}

// Sorted orders findings most serious first, keeping each severity in the
// order it was found so a reader sees the run's own sequence.
func (r *Report) Sort() {
	r.Findings = SortedBy(r.Findings, func(a, b Finding) int {
		return SevRank(a.Severity) - SevRank(b.Severity)
	})
}

// ReportFlags are what a checking verb takes: which severity fails, whether
// to keep a history, and cli's JSON pair. Registered together so every such
// verb on the stack is asked the same way.
func ReportFlags(fs *flag.FlagSet) {
	JSONFlags(fs)
	fs.String("fail-on", SevError, "the least serious finding that still fails: `error|warning|info`")
	fs.String("record", "", "keep this run in `DIR`, and say what changed since the last one")
	fs.Var(new(Bool), "quiet", "no progress on stderr; the report still goes to stdout")
}

// CheckReportFlags rejects a bad value before any work happens. A crawl takes
// seconds to minutes, and failing afterwards on a typo in --fail-on wastes
// every one of them.
func (c Call) CheckReportFlags() error {
	s := c.Value("fail-on")
	if s == "" || slices.Contains(Severities, s) {
		return nil
	}
	if near := Nearest(s, Severities); near != "" {
		return c.Usagef("--fail-on %q — did you mean %q?", s, near)
	}
	return c.Usagef("--fail-on %q: want %s", s, EitherOr(Severities))
}

// Record writes the run to DIR as a timestamped file and refreshes
// latest.json, so a series of runs is a history that can be diffed. The
// timestamp is UTC and sorts lexicographically, which is what makes the
// directory readable in order.
func (c Call) Record(r *Report) (string, error) {
	dir := c.Value("record")
	if dir == "" {
		return "", nil
	}
	if !filepath.IsAbs(dir) {
		root, err := root()
		if err == nil {
			dir = filepath.Join(root, dir)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.json", r.Tool, r.RanAt.Format("2006-01-02T150405Z")))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, os.WriteFile(filepath.Join(dir, "latest.json"), data, 0o644)
}

// History is every run recorded in --record, oldest first, minus the one
// being written now.
//
// The whole history rather than the last one, because a loop that improves
// something needs to know what it has already failed at. Comparing against a
// single previous run says a finding is "unchanged", which is the same word
// for a thing nobody has tried yet and a thing six attempts have not moved.
// Those want opposite decisions.
func (c Call) History(exclude string) []*Report {
	dir := c.Value("record")
	if dir == "" {
		return nil
	}
	if !filepath.IsAbs(dir) {
		if root, err := root(); err == nil {
			dir = filepath.Join(root, dir)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		// latest.json is a copy of one of the others and would count twice.
		if n != "latest.json" && filepath.Ext(n) == ".json" && filepath.Join(dir, n) != exclude {
			names = append(names, n)
		}
	}
	// The name carries the time it ran, so sorting the names is sorting the
	// runs — no file needs opening to put them in order.
	var out []*Report
	for _, n := range Sorted(names) {
		data, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		var r Report
		if json.Unmarshal(data, &r) == nil {
			out = append(out, &r)
		}
	}
	return out
}

// Previous is the most recent recorded run before this one, for the drift
// report. Nothing recorded yet is not an error: the first run has nothing to
// differ from.
func (c Call) Previous(r *Report, exclude string) (*Report, bool) {
	history := c.History(exclude)
	if len(history) == 0 {
		return nil, false
	}
	return history[len(history)-1], true
}

// Standing is how many recorded runs in a row have carried this finding,
// counting back from the most recent. 1 means it is new.
//
// This is the number that tells a loop what to do next. A finding standing
// at 1 is worth the obvious fix; one standing at 6 has had the obvious fix
// tried five times, and doing it again is the definition of not learning.
// Without it every run starts from nothing and re-attempts what the last one
// already failed at — which is what an evolving system must not do.
func Standing(history []*Report, f Finding) int {
	n := 0
	for _, h := range slices.Backward(history) {
		if !slices.ContainsFunc(h.Findings, func(had Finding) bool { return same(had, f) }) {
			break
		}
		n++
	}
	return n + 1 // the run being reported now
}

// same is when two findings from different runs are the one finding. The
// message is part of it because one id covers many subjects — a broken link
// is "broken-link" whichever link it is.
func same(a, b Finding) bool {
	return a.Tool == b.Tool && a.ID == b.ID && a.Message == b.Message
}

// Drift is what changed since a previous run: what was fixed, and what is
// new. The point of keeping a history is seeing movement, not totals.
func Drift(prev, cur *Report) (fixed, arrived []Finding) {
	// Through the same rule Standing uses. Written out here as well, the two
	// could disagree about whether a finding is the same finding, and then a
	// report would say a thing was fixed and count it as standing.
	held := func(in []Finding) func(Finding) bool {
		return func(f Finding) bool {
			return !slices.ContainsFunc(in, func(had Finding) bool { return same(had, f) })
		}
	}
	return Filter(prev.Findings, held(cur.Findings)), Filter(cur.Findings, held(prev.Findings))
}

// Parallel runs each unit of work and returns their results in the order they
// were given, whatever order they finished in — so a report is the same bytes
// on every run and a diff of two runs is about the subject, not the schedule.
//
// A panic in one unit is recorded against that unit rather than taking down
// the run: one badly-behaved tool must not cost the answers from the others.
func Parallel[T any](jobs int, work []func() T, onPanic func(i int, v any) T) []T {
	if jobs < 1 {
		jobs = 1
	}
	out := make([]T, len(work))
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	for i, fn := range work {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			defer func() {
				if v := recover(); v != nil && onPanic != nil {
					out[i] = onPanic(i, v)
				}
			}()
			out[i] = fn()
		})
	}
	wg.Wait()
	return out
}

// Or is the first non-empty of the two: the default a caller falls back to
// when a field, a flag or a tool's answer is blank. Three packages had
// written it, and cmp.Or in the standard library wants comparable ordered
// values rather than this one narrow case, which is the one that keeps
// coming up.
func Or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// Measured is one named part of a report: the step that describes the run,
// and whatever it found.
//
// Four reports in this tree built this by hand and each wrote the same five
// lines around it — start a clock, call the thing, stop the clock, fill a
// Step, then either NotRun with the error or Ran and attribute the findings.
// The fifth line was the one that varied, and only by being forgotten:
// findings attributed to the part in one place and not in another.
type Measured struct {
	Step     Step
	Findings []Finding
}

// Measure runs one part of a report and times it.
//
// look returns what it found, one line saying what it looked at, and whether
// it could look at all. A part that could not run is not a failure of the
// report: it is a step with a reason, and the rest still run.
func Measure(name, provides string, look func() ([]Finding, string, error)) Measured {
	at := time.Now()
	found, covered, err := look()
	took := time.Since(at)
	step := Step{
		Name: name, Provides: provides, Covered: covered,
		Took: Took(took), TookMs: took.Milliseconds(), Findings: len(found),
	}
	if err != nil {
		step.Status, step.Note = StatusSkipped, err.Error()
	}
	// Findings are kept whether or not look also returned an error, because a
	// look that returns both is saying "this could not be done, and here is
	// why in the report's own terms" — which is what a zone nothing answers
	// for is. Dropping them left the step advertising a note the report did
	// not hold, so `dev fronting` on an unknown host printed "1 note" and
	// "nothing to report" two lines apart.
	//
	// Every finding says which part found it. Left to each caller this was
	// done in some and not others, so a report could name a problem without
	// naming what noticed it.
	for i := range found {
		if found[i].Tool == "" {
			found[i].Tool = name
		}
	}
	return Measured{Step: step, Findings: found}
}

// Record files what Measure produced.
func (r *Report) Record(m Measured) {
	for _, f := range m.Findings {
		r.Add(f)
	}
	if m.Step.Status == StatusSkipped {
		r.NotRun(m.Step, m.Step.Note)
		return
	}
	r.Ran(m.Step)
}

// Part is one named piece of a report, for the common case where the parts
// are known and each is just a function. Declaring them reads as a list of
// what is checked, which is what a reader of the report will see.
type Part struct {
	Name     string
	Provides string
	Look     func() ([]Finding, string, error)
}

// Parts runs a declared set and records each.
func Parts(r *Report, jobs int, parts []Part) {
	Gather(r, jobs, parts,
		func(p Part) string { return p.Name },
		func(p Part) Measured { return Measure(p.Name, p.Provides, p.Look) })
}

// Gather runs every part and records them in the order given, however they
// were scheduled — so a report reads the same whether it was run one at a
// time or all at once, which is what makes two runs comparable.
//
// jobs is how many at once: one for parts that must not overlap, len(parts)
// for independent questions to an API.
//
// name says what to call a part that panics, because the thing that would
// have named it is the thing that did not finish. Without it the report says
// the whole struct, which is how a checker's bad afternoon became forty lines
// of Go in a step's name.
func Gather[T any](r *Report, jobs int, parts []T, name func(T) string, run func(T) Measured) {
	if jobs < 1 {
		jobs = 1
	}
	work := Map(parts, func(p T) func() Measured {
		return func() Measured { return run(p) }
	})
	// A part that panics is recorded as not run rather than taking the report
	// down with it: one tool's bad afternoon is not a reason to learn nothing
	// about the rest.
	for _, m := range Parallel(jobs, work, func(i int, v any) Measured {
		return Measured{Step: Step{Name: name(parts[i]), Status: StatusSkipped,
			Note: fmt.Sprintf("panicked: %v", v)}}
	}) {
		r.Record(m)
	}
}

// Write prints a report the way every command on this stack prints one: what
// was looked at, then what was found, then the verdict.
//
// Four commands wrote this themselves and no two agreed. One showed the tool
// that found a thing and another did not; one printed the fix and another
// dropped it; the column widths differed by a character each time. None of
// that was a decision — it was four people writing the same paragraph from
// memory, and a reader moving between two of them had to notice that the
// shapes were nearly but not quite the same.
//
// A command that genuinely needs a different shape still writes its own; what
// this removes is having to.
func (c Call) Write(r *Report) {
	// Sized to what is actually there. A fixed width is right until a report
	// is about domains rather than about three checks with short names, and
	// then every line is a character out from its neighbour.
	wide := Widest(r.Steps, func(s Step) string { return s.Name })
	for _, s := range r.Steps {
		mark := " "
		if s.Status == StatusSkipped {
			mark = "!"
		}
		fmt.Fprintf(c.Stdout, "%s %-*s %7s  %-34s %s\n",
			mark, wide, s.Name, s.Took, Or(s.Covered, s.Note), Plural(s.Findings, "note"))
	}
	c.Findings(r)
	fmt.Fprintf(c.Stdout, "%s in %s: %s\n", r.Outcome, r.Took, r.Summary())
}

// Findings prints what a report found, which is the half every command shows
// the same way even when the steps above it differ.
//
// seo lists a sub-report per checker and fronting does not; both list a fault
// the same way, and did so in two places that had drifted — one dropped the
// fix, another dropped where it was found.
func (c Call) Findings(r *Report) {
	if len(r.Findings) == 0 {
		return
	}
	fmt.Fprintln(c.Stdout)
	for _, f := range r.Findings {
		// The tool is named when it is not already obvious from the message:
		// "missing-canonical (kitsune)" is worth a reader's attention and
		// "serves-nothing (amplifycms.com)" repeats the line above it.
		who := ""
		if f.Tool != "" && !strings.Contains(f.Message, f.Tool) {
			who = " (" + f.Tool + ")"
		}
		fmt.Fprintf(c.Stdout, "%-8s %s%s\n  %s\n", f.Severity, f.ID, who, f.Message)
		if f.Fix != "" {
			fmt.Fprintf(c.Stdout, "  fix: %s\n", f.Fix)
		}
		if f.Where != "" {
			fmt.Fprintf(c.Stdout, "  on: %s\n", f.Where)
		}
		fmt.Fprintln(c.Stdout)
	}
}

// Reported opens a report, runs body against it and finishes it: the flags
// checked, the clock started, the verdict decided and the file written.
//
// Four verbs wrote those five lines themselves, which is four chances to
// forget one — and CheckReportFlags is the one that matters, because without
// it --fail-on takes a severity nobody spells and the gate silently passes.
func (c Call) Reported(name, target string, body func(*Report) error) error {
	if err := c.CheckReportFlags(); err != nil {
		return err
	}
	started := time.Now()
	r := NewReport(name, target)
	if err := body(r); err != nil {
		return err
	}
	return c.Finish(r, started, c.Write)
}
