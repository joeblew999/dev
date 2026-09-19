// The report is what every checking verb on the stack answers in, so what it
// promises is held here rather than inferred from the one verb that happens
// to exercise it.
package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func reportCall(t *testing.T, args ...string) Call {
	t.Helper()
	fs := Flags("tool check", io.Discard)
	ReportFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return Call{Verb: "tool check", Flags: fs, Stdout: io.Discard, Stderr: io.Discard}
}

// --fail-on decides what a gate means, so it is the one value most worth
// getting wrong quietly: a typo that behaved like "info" would fail every run
// and look like the checker was broken.
func TestFailOnDecidesTheOutcome(t *testing.T) {
	for _, tc := range []struct {
		failOn string
		worst  string
		want   string
	}{
		{SevError, SevError, "fail"},
		{SevError, SevWarning, "pass"},
		{SevWarning, SevWarning, "fail"},
		{SevWarning, SevInfo, "pass"},
		{SevInfo, SevInfo, "fail"},
		{"", SevError, "fail"},   // empty means errors fail
		{"", SevWarning, "pass"}, // and nothing else does
	} {
		r := NewReport("tool", "target")
		r.Add(Finding{Severity: tc.worst, ID: "x"})
		r.Done(time.Now(), tc.failOn)
		if r.Outcome != tc.want {
			t.Errorf("--fail-on %q with a %s: %q; want %q", tc.failOn, tc.worst, r.Outcome, tc.want)
		}
	}
	// A report with nothing in it passes whatever the gate is set to.
	r := NewReport("tool", "target")
	r.Done(time.Now(), SevInfo)
	if r.Outcome != "pass" {
		t.Errorf("empty report = %q; want pass", r.Outcome)
	}
}

// A bad --fail-on is caught before any work happens, and says what was meant.
func TestCheckReportFlagsRejectsATypo(t *testing.T) {
	err := reportCall(t, "--fail-on", "wornings").CheckReportFlags()
	if err == nil || !strings.Contains(err.Error(), `did you mean "warning"`) {
		t.Errorf("err = %v; want it to suggest warning", err)
	}
	if err := reportCall(t, "--fail-on", SevWarning).CheckReportFlags(); err != nil {
		t.Errorf("a valid severity was rejected: %v", err)
	}
	// Anything unlike a severity is told what the choices are rather than
	// being offered the nearest of three unrelated words.
	err = reportCall(t, "--fail-on", "everything").CheckReportFlags()
	if err == nil || !strings.Contains(err.Error(), "want error, warning or info") {
		t.Errorf("err = %v; want the three choices", err)
	}
}

func TestDoneCountsAndSorts(t *testing.T) {
	r := NewReport("tool", "target")
	r.Add(Finding{Tool: "a", Severity: SevInfo, ID: "i"})
	r.Add(Finding{Tool: "b", Severity: SevError, ID: "e"})
	r.Add(Finding{Tool: "a", Severity: SevWarning, ID: "w"})
	r.Sort()
	r.Done(time.Now(), SevError)

	if got := Map(r.Findings, func(f Finding) string { return f.ID }); !slices.Equal(got, []string{"e", "w", "i"}) {
		t.Errorf("order = %v; want the most serious first", got)
	}
	if r.BySeverity[SevError] != 1 || r.ByTool["a"] != 2 {
		t.Errorf("counts = %v / %v; want one error and two from a", r.BySeverity, r.ByTool)
	}
	if r.Took == "" || r.TookMs < 0 {
		t.Errorf("no elapsed time recorded: %q / %d", r.Took, r.TookMs)
	}
}

// A step that did not run says so, with why and what it would have given —
// because "did not run" and "found nothing" read the same and mean opposite
// things.
func TestNotRunIsNeverSilent(t *testing.T) {
	r := NewReport("tool", "target")
	r.Ran(Step{Name: "ran"})
	r.NotRun(Step{Name: "missing", Provides: "what you would learn", Cost: "~2s"}, "not installed")

	if r.Steps[0].Status != StatusOK {
		t.Errorf("a step that ran = %q; want ok", r.Steps[0].Status)
	}
	s := r.Steps[1]
	if s.Status != StatusSkipped || s.Note != "not installed" || s.Provides == "" {
		t.Errorf("skipped step = %+v; want why and what it gives", s)
	}
}

// Parallel is what makes two runs of the same subject comparable: the work
// finishes in any order and the results come back in the order given.
func TestParallelKeepsTheOrderItWasGiven(t *testing.T) {
	work := make([]func() int, 50)
	for i := range work {
		work[i] = func() int {
			// Later items finish first, so anything order-dependent breaks.
			time.Sleep(time.Duration(len(work)-i) * time.Millisecond / 10)
			return i
		}
	}
	got := Parallel(8, work, nil)
	want := make([]int, len(work))
	for i := range want {
		want[i] = i
	}
	if !slices.Equal(got, want) {
		t.Errorf("Parallel returned %v; want the order it was given", got)
	}
}

// A panic in one unit is that unit's problem, not the run's: the others still
// answer, and the one that failed says what happened.
func TestParallelSurvivesAPanic(t *testing.T) {
	work := []func() string{
		func() string { return "first" },
		func() string { panic("a parser gave up") },
		func() string { return "third" },
	}
	got := Parallel(3, work, func(i int, v any) string {
		return "recovered: " + v.(string)
	})
	if !slices.Equal(got, []string{"first", "recovered: a parser gave up", "third"}) {
		t.Errorf("got %v; want the panic recorded against its own unit", got)
	}
}

// Findings arrive from several goroutines at once, so the report has to take
// them safely. Run with -race, this is the test that says so.
func TestReportTakesFindingsConcurrently(t *testing.T) {
	r := NewReport("tool", "target")
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			r.Add(Finding{Tool: "t", Severity: SevInfo, ID: string(rune('a' + i%26))})
			r.Ran(Step{Name: "s"})
		})
	}
	wg.Wait()
	if len(r.Findings) != 100 || len(r.Steps) != 100 {
		t.Errorf("got %d findings and %d steps; want 100 of each", len(r.Findings), len(r.Steps))
	}
}

// A history is only worth keeping if it says what moved. Record writes a run,
// Previous finds the one before, and Drift is the difference.
func TestRecordThenDrift(t *testing.T) {
	dir := t.TempDir()
	c := reportCall(t, "--record", dir)

	first := NewReport("tool", "target")
	first.Add(Finding{Tool: "a", Severity: SevError, ID: "was-broken", Message: "m"})
	first.Add(Finding{Tool: "a", Severity: SevWarning, ID: "still-there", Message: "m"})
	first.Done(time.Now(), SevError)
	firstPath, err := c.Record(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "latest.json")); err != nil {
		t.Errorf("latest.json was not refreshed: %v", err)
	}

	// A second run a moment later, so the timestamps differ.
	second := NewReport("tool", "target")
	second.RanAt = second.RanAt.Add(time.Second)
	second.Add(Finding{Tool: "a", Severity: SevWarning, ID: "still-there", Message: "m"})
	second.Add(Finding{Tool: "a", Severity: SevError, ID: "new-problem", Message: "m"})
	second.Done(time.Now(), SevError)
	secondPath, err := c.Record(second)
	if err != nil {
		t.Fatal(err)
	}

	prev, ok := c.Previous(second, secondPath)
	if !ok {
		t.Fatal("the previous run was not found")
	}
	if prev.RanAt.Unix() != first.RanAt.Unix() {
		t.Errorf("previous is %v; want the first run at %v", prev.RanAt, first.RanAt)
	}
	fixed, arrived := Drift(prev, second)
	if len(fixed) != 1 || fixed[0].ID != "was-broken" {
		t.Errorf("fixed = %v; want was-broken", Map(fixed, func(f Finding) string { return f.ID }))
	}
	if len(arrived) != 1 || arrived[0].ID != "new-problem" {
		t.Errorf("arrived = %v; want new-problem", Map(arrived, func(f Finding) string { return f.ID }))
	}
	// The first run in a directory has nothing before it, and that is not an
	// error: day one must not need a manual step.
	fresh := reportCall(t, "--record", t.TempDir())
	only := NewReport("tool", "target")
	onlyPath, err := fresh.Record(only)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fresh.Previous(only, onlyPath); ok {
		t.Error("a run before the first was reported")
	}
	// What was written is the report, readable back.
	data, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("what was recorded is not JSON: %v", err)
	}
	if back.Outcome != "fail" || len(back.Findings) != 2 {
		t.Errorf("read back %s with %d findings; want fail with 2", back.Outcome, len(back.Findings))
	}
}

// Without --record nothing is written and nothing is looked for.
func TestRecordIsOptional(t *testing.T) {
	c := reportCall(t)
	path, err := c.Record(NewReport("tool", "target"))
	if err != nil || path != "" {
		t.Errorf("Record without --record = %q, %v; want nothing done", path, err)
	}
	if _, ok := c.Previous(NewReport("tool", "target"), ""); ok {
		t.Error("a previous run was found with no --record")
	}
}

func TestSevRankOrdersAndRejects(t *testing.T) {
	if !(SevRank(SevError) < SevRank(SevWarning) && SevRank(SevWarning) < SevRank(SevInfo)) {
		t.Error("severities are out of order")
	}
	// An unknown severity sorts last and must never satisfy a gate.
	r := NewReport("tool", "target")
	r.Add(Finding{Severity: "spicy"})
	if r.Failed(SevInfo) {
		t.Error("an unknown severity satisfied --fail-on info")
	}
}

func TestReportFlagsAreRegistered(t *testing.T) {
	fs := Flags("tool check", io.Discard)
	ReportFlags(fs)
	for _, name := range []string{"json", "out", "fail-on", "record", "quiet"} {
		if fs.Lookup(name) == nil {
			t.Errorf("ReportFlags did not register --%s", name)
		}
	}
	// The default is the one a gate wants: errors fail, warnings do not.
	if got := fs.Lookup("fail-on").DefValue; got != SevError {
		t.Errorf("--fail-on defaults to %q; want %q", got, SevError)
	}
}

var _ = flag.ErrHelp

// Finish is the tail every checking verb shares, so what it promises is held
// here rather than in whichever verb was written first.
func TestFinishIsTheWholeTail(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	c := reportCall(t, "--record", dir, "--quiet")
	c.Stdout = &out

	r := NewReport("tool", "a target")
	r.Add(Finding{Tool: "t", Severity: SevWarning, ID: "w", Message: "m"})
	wrote := false
	err := c.Finish(r, time.Now(), func(*Report) { wrote = true })

	// A warning does not fail the default gate, and the prose renderer runs
	// because nothing asked for JSON.
	if err != nil {
		t.Errorf("a warning failed the default gate: %v", err)
	}
	if !wrote {
		t.Error("the prose renderer did not run")
	}
	if r.Outcome != "pass" || r.BySeverity[SevWarning] != 1 {
		t.Errorf("report = %s %v; want pass with one warning", r.Outcome, r.BySeverity)
	}
	if _, err := os.Stat(filepath.Join(dir, "latest.json")); err != nil {
		t.Errorf("--record wrote nothing: %v", err)
	}

	// An error fails, and a verb with a better sentence than the counts says
	// it instead.
	bad := NewReport("tool", "a target")
	bad.Add(Finding{Severity: SevError, ID: "e"})
	bad.Fail = "fix it with: some command"
	if err := c.Finish(bad, time.Now(), nil); err == nil || err.Error() != "fix it with: some command" {
		t.Errorf("err = %v; want the report's own sentence", err)
	}

	// Without one, the counts are the sentence.
	plain := NewReport("tool", "a target")
	plain.Add(Finding{Severity: SevError, ID: "e"})
	err = c.Finish(plain, time.Now(), nil)
	if err == nil || !strings.Contains(err.Error(), "1 error") {
		t.Errorf("err = %v; want the counts", err)
	}

	// Asking for JSON means the prose renderer is not called at all.
	js := reportCall(t, "--json")
	var jsOut bytes.Buffer
	js.Stdout = &jsOut
	called := false
	if err := js.Finish(NewReport("tool", "t"), time.Now(), func(*Report) { called = true }); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("the prose renderer ran for a JSON answer")
	}
	if !strings.Contains(jsOut.String(), `"outcome"`) {
		t.Errorf("stdout is not the report: %q", jsOut.String())
	}
}
