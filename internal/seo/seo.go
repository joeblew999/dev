// Package seo checks a deployed URL against what Google Search asks of a
// page, by running the checkers that already do it well and merging what they
// say into one report.
//
// It is deliberately thin. Running a program is cli/tool's, decoding what it
// printed is cli.DecodeJSON's, and answering in JSON to stdout, to .reports/
// and to a sub-report per tool is cli's — so what lives here is only what is
// true of SEO: which checkers, how to read each one, and what a person should
// do about what they find.
//
// No checker is made to share a shape. Each says what it found in its own
// JSON, which is written beside the report and linked from it; this package
// takes from each only what every checker has in common — an issue, where it
// is, and how to fix it — and leaves the rest where a reader can follow it.
package seo

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/cli/tool"
	"github.com/joeblew999/dev/internal/seo/checkers"
)

//go:embed usage.md
var Usage string

// Subs are the three things this verb does: write the files a site needs,
// validate them with no network, and check a deployed URL with the checkers.
// Writing and validating share one declaration, so an artifact cannot pass
// when it is written and fail when it is checked.
var Subs = map[string]cli.Verb{
	"write":    {Run: runWrite, Args: "DIR", Flags: WriteFlags, Desc: "write sitemap.xml, robots.txt and head.html, then validate them"},
	"validate": {Run: runValidate, Args: "DIR", Flags: ValidateFlags, Desc: "check those files on disk, with no network at all"},
	"check":    {Run: runCheck, Args: "URL", Flags: CheckFlags, Desc: "run every checker against a deployed URL and merge what they say"},
	"can":      {Run: runList, Flags: cli.JSONFlags, Desc: "what this verb can write and check, what each needs, and whether it is installed"},
}

// CheckFlags are what `check` takes: how far to crawl, which checkers, and
// cli's report flags.
func CheckFlags(fs *flag.FlagSet) {
	fs.Int("max-pages", 20, "stop crawling after this many `PAGES`")
	pickFlags(fs)
	fs.Int("jobs", 0, "checkers to run at once (default: all of them)")
	cli.ReportFlags(fs)
}

// pickFlags choose which of a registry's entries run. The same two words for
// checkers and for writers, because they mean the same thing.
func pickFlags(fs *flag.FlagSet) {
	fs.String("only", "", "run only these, by `NAME,NAME`")
	fs.String("skip", "", "run everything except these, by `NAME,NAME`")
}

// WriteFlags are what the page says about itself.
func WriteFlags(fs *flag.FlagSet) {
	pickFlags(fs)
	fs.String("url", "", "the page the head fragment describes, and the site the sitemap covers")
	fs.String("title", "", "the `TEXT` Search shows as the headline")
	fs.String("desc", "", "the `TEXT` Search shows under it")
	fs.String("image", "", "the `URL` a shared link previews")
	fs.String("urls", "", "a `FILE` of URLs, one per line, for the sitemap")
	cli.ReportFlags(fs)
}

// ValidateFlags are what `validate` takes.
func ValidateFlags(fs *flag.FlagSet) {
	pickFlags(fs)
	fs.String("url", "", "the site these files belong to; read from the files when not given")
	cli.ReportFlags(fs)
}

// start is what every subcommand opens with: the flags checked before any
// work, the selection resolved against a registry, and a report begun. Three
// of them had written it out.
func start(c cli.Call, target string, of []string) (*cli.Report, *picked, time.Time, error) {
	if err := c.CheckReportFlags(); err != nil {
		return nil, nil, time.Time{}, err
	}
	pick, err := selection(c, of)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	return cli.NewReport("seo", target), pick, time.Now(), nil
}

// writerNames and checkerNames are what --only and --skip are resolved
// against, per subcommand.
func writerNames() []string {
	return names(writers, func(w Writer) string { return w.Name })
}

func checkerNames() []string {
	return names(checkers.All, func(ch checkers.Checker) string { return ch.Name })
}

// runCheck is `dev seo check URL`.
func runCheck(c cli.Call) error {
	maxPages, err := c.ValueAs("max-pages", strconv.Atoi)
	if err != nil {
		return c.Usagef("--max-pages wants a number: %v", err)
	}
	rep, pick, started, err := start(c, c.Args[0], checkerNames())
	if err != nil {
		return err
	}
	Audit(c, rep, c.Args[0], maxPages, pick)
	return c.Finish(rep, started, func(r *cli.Report) { write(c, r) })
}

// onDir is what write and validate both are: resolve which writers run,
// begin a report, do the one thing that differs, and finish. Written out
// twice, the two drifted the moment either gained a flag.
func onDir(c cli.Call, do func(*cli.Report, *picked) error) error {
	rep, pick, started, err := start(c, c.Dir, writerNames())
	if err != nil {
		return err
	}
	if err := do(rep, pick); err != nil {
		return err
	}
	return c.Finish(rep, started, func(r *cli.Report) { write(c, r) })
}

// runWrite is `dev seo write DIR`.
func runWrite(c cli.Call) error {
	url := c.Value("url")
	if url == "" {
		return c.Usagef("--url says which site these files are for")
	}
	urls, err := sitemapURLs(c, url)
	if err != nil {
		return err
	}
	return onDir(c, func(rep *cli.Report, pick *picked) error {
		return Write(c, c.Dir, Site{
			Origin: originOf(url), URL: url, Now: time.Now().UTC(), URLs: urls,
			Title: c.Value("title"), Desc: c.Value("desc"), Image: c.Value("image"),
		}, rep, pick)
	})
}

// runValidate is `dev seo validate DIR`.
func runValidate(c cli.Call) error {
	return onDir(c, func(rep *cli.Report, pick *picked) error {
		return Validate(c.Dir, originOf(c.Value("url")), rep, pick)
	})
}

// sitemapURLs is what the sitemap should list: an explicit file when given —
// faster, reproducible, and it reaches pages nothing links to — otherwise the
// one page named.
func sitemapURLs(c cli.Call, url string) ([]string, error) {
	file := c.Value("urls")
	if file == "" {
		return []string{url}, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, c.Usagef("--urls: %v", err)
	}
	urls := cli.Filter(cli.Lines(string(data)), func(l string) bool {
		l = strings.TrimSpace(l)
		return l != "" && !strings.HasPrefix(l, "#")
	})
	if len(urls) == 0 {
		return nil, c.Usagef("--urls: %s lists no URLs", file)
	}
	return cli.Map(urls, strings.TrimSpace), nil
}

// Audit runs every selected checker against url and merges what they found.
//
// They run at once and merge in registry order, so the report is the same
// bytes however they were scheduled — which is what makes two runs comparable.
// A checker that fails is recorded with the reason and the rest still run:
// one tool missing is not a reason to learn nothing about the page.
func Audit(c cli.Call, rep *cli.Report, url string, maxPages int, pick *picked) {
	var run []checkers.Checker
	for _, ch := range checkers.All {
		if why := pick.skipped(ch.Name); why != "" {
			rep.NotRun(cli.Step{Name: ch.Name, Provides: ch.Provides, Cost: ch.Cost,
				Requires: ch.Pin}, why)
			continue
		}
		run = append(run, ch)
	}
	work := cli.Map(run, func(ch checkers.Checker) func() result {
		return func() result { return run1(ch, c, url, maxPages) }
	})
	jobs, _ := c.ValueAs("jobs", strconv.Atoi)
	if jobs <= 0 {
		jobs = len(work)
	}
	results := cli.Parallel(jobs, work, func(i int, v any) result {
		// A panic in one checker's reader is that checker's problem, not the
		// run's: record it against the tool and keep the other answers.
		return result{step: cli.Step{Name: run[i].Name, Status: cli.StatusSkipped,
			Note: fmt.Sprintf("panicked: %v", v)}}
	})
	for _, res := range results {
		if res.step.Status == cli.StatusSkipped {
			rep.NotRun(res.step, res.step.Note)
		} else {
			rep.Ran(res.step)
		}
		for _, f := range res.findings {
			// Which of this command's writers closes the gap, so the report
			// is a route to a fix and not only a list of faults. Empty when
			// no file fixes it: a broken link is content, not a tag.
			f.FixedBy = fixedBy(f.ID)
			rep.Add(f)
		}
	}
}

// result is one checker's run: how it went, and what it found.
type result struct {
	step     cli.Step
	findings []cli.Finding
}

// names is the name of each thing in a registry, for --only and --skip and
// for the listing. A tiny generic so both registries answer it the same way
// without either one growing an interface to satisfy.
func names[T any](of []T, name func(T) string) []string { return cli.Map(of, name) }

// picked is what --only and --skip resolved to. A name that is not a checker
// names itself rather than silently running nothing.
type picked struct{ only, skip map[string]bool }

func selection(c cli.Call, names []string) (*picked, error) {
	read := func(flag string) (map[string]bool, error) {
		out := map[string]bool{}
		for _, n := range cli.Lines(strings.ReplaceAll(c.Value(flag), ",", "\n")) {
			if n = strings.TrimSpace(n); n == "" {
				continue
			}
			if !cli.ToSet(names)[n] {
				if near := cli.Nearest(n, names); near != "" {
					return nil, c.Usagef("--%s: no checker %q — did you mean %q?", flag, n, near)
				}
				return nil, c.Usagef("--%s: no checker %q; they are: %s", flag, n, strings.Join(names, ", "))
			}
			out[n] = true
		}
		return out, nil
	}
	only, err := read("only")
	if err != nil {
		return nil, err
	}
	skip, err := read("skip")
	if err != nil {
		return nil, err
	}
	return &picked{only: only, skip: skip}, nil
}

// skipped says why a checker will not run, or "" when it will.
func (p *picked) skipped(name string) string {
	switch {
	case p == nil:
		return ""
	case p.skip[name]:
		return "excluded by --skip"
	case len(p.only) > 0 && !p.only[name]:
		return "not in --only"
	}
	return ""
}

// run is one checker: ask it, keep everything it said, and take from it the
// part this report shares.
func run1(ch checkers.Checker, c cli.Call, url string, maxPages int) result {
	ask := checkers.Ask{URL: url, Pages: maxPages}
	if ch.Fetch != "" {
		path, cleanup, err := download(originOf(url) + ch.Fetch)
		if err != nil {
			return result{step: cli.Step{Name: ch.Name, Status: cli.StatusSkipped,
				Note: err.Error(), Provides: ch.Provides, Cost: ch.Cost, Requires: ch.Pin}}
		}
		defer cleanup()
		ask.File = path
	}
	res, err := tool.Run(ch.Name, ch.Pin, ch.Args(ask)...)
	step := cli.Step{Name: ch.Name, Provides: ch.Provides, Cost: ch.Cost,
		Requires: ch.Pin, Took: cli.Took(res.Took), TookMs: res.Took.Milliseconds()}
	// A checker that could not run, and one whose output could not be read,
	// are the same to a reader: it did not answer, and here is why.
	gaveUp := func(err error) result {
		step.Status, step.Note = cli.StatusSkipped, err.Error()
		return result{step: step}
	}
	if err != nil {
		return gaveUp(err)
	}
	// Written before it is read, so a checker this package cannot parse still
	// leaves everything it said where a person can look.
	if path, err := c.SubReport(ch.Name, res.Out); err == nil {
		step.Report = path
	}
	found, err := ch.Read(res)
	if err != nil {
		return gaveUp(err)
	}
	step.Findings, step.Covered = len(found.Issues), found.Covered()
	return result{step: step, findings: found.Issues}
}

// write is the human answer: what ran and where its own output is, every
// issue with its fix, what did not run and why, then where that leaves you.
func write(c cli.Call, r *cli.Report) {
	fmt.Fprintf(c.Stdout, "%s\n", r.Target)
	for _, s := range r.Steps {
		if s.Status != cli.StatusOK {
			continue
		}
		fmt.Fprintf(c.Stdout, "  %-13s %7s  %-44s", s.Name, s.Took, short(s.Covered, 44))
		if s.Findings > 0 {
			fmt.Fprintf(c.Stdout, " %s", cli.Plural(s.Findings, "finding"))
		}
		if s.Report != "" {
			fmt.Fprintf(c.Stdout, "  → %s", here(s.Report))
		}
		fmt.Fprintln(c.Stdout)
	}
	fmt.Fprintln(c.Stdout)
	// One line per finding, each naming the checker that found it. Several
	// checkers reporting the same fault is fine and worth seeing: they phrase
	// it differently, and where they agree the problem is not in doubt.
	for _, f := range r.Findings {
		fmt.Fprintf(c.Stdout, "%-8s %s (%s)\n  %s\n", f.Severity, f.ID, f.Tool, f.Message)
		if f.Fix != "" {
			fmt.Fprintf(c.Stdout, "  fix: %s\n", f.Fix)
		}
		if f.Where != "" {
			fmt.Fprintf(c.Stdout, "  on: %s\n", f.Where)
		}
		fmt.Fprintln(c.Stdout)
	}
	notRun(c, r)
	fixable(c, r)
	fmt.Fprintf(c.Stdout, "%s in %s: %s, %s\n", r.Outcome, r.Took,
		cli.Plural(r.BySeverity[cli.SevError], "error"), cli.Plural(r.BySeverity[cli.SevWarning], "warning"))
}

// short keeps a column a column. What is cut is in the JSON in full.
func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// here is a path as the person running this would type it: relative to where
// they are, when that is shorter than the absolute one.
func here(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(wd, path)
	if err != nil || len(rel) >= len(path) || strings.HasPrefix(rel, "../../") {
		return path
	}
	return rel
}

// fixable is the route out: how many of these findings this command can fix
// by writing the files, which files those are, and the line that does it.
// The rest are said to be content or upstream, because knowing a finding is
// not yours to fix is worth as much as knowing how to fix it.
func fixable(c cli.Call, r *cli.Report) {
	byWriter := cli.CountBy(cli.Filter(r.Findings, func(f cli.Finding) bool {
		return f.FixedBy != ""
	}), func(f cli.Finding) string { return f.FixedBy })
	if len(byWriter) == 0 {
		return
	}
	n := 0
	for _, c := range byWriter {
		n += c
	}
	fmt.Fprintf(c.Stdout, "%d of %d findings are files this can write:\n", n, len(r.Findings))
	for _, name := range cli.SortedKeys(byWriter) {
		for _, w := range writers {
			if w.Name == name {
				fmt.Fprintf(c.Stdout, "  %-10s %-16s fixes %d\n", name, w.Produces.Name, byWriter[name])
			}
		}
	}
	fmt.Fprintf(c.Stdout, "\n  dev seo write <dir> --url %s --title \"...\" --desc \"...\" --image ...\n\n", r.Target)
}

// notRun says what this run did not do and how to enable it. A report that
// quietly omits what it skipped reads as complete when it is not.
func notRun(c cli.Call, r *cli.Report) {
	skipped := cli.Filter(r.Steps, func(s cli.Step) bool { return s.Status != cli.StatusOK })
	if len(skipped) == 0 {
		return
	}
	fmt.Fprintf(c.Stdout, "not run — available if you supply what it needs\n\n")
	for _, s := range skipped {
		fmt.Fprintf(c.Stdout, "  %s\n", s.Name)
		fmt.Fprintf(c.Stdout, "      why     %s\n", s.Note)
		if s.Provides != "" {
			fmt.Fprintf(c.Stdout, "      gives   %s\n", s.Provides)
		}
		if s.Requires != "" {
			fmt.Fprintf(c.Stdout, "      needs   %s\n", s.Requires)
		}
		if s.Cost != "" {
			fmt.Fprintf(c.Stdout, "      costs   %s\n", s.Cost)
		}
	}
	fmt.Fprintln(c.Stdout)
}

// download fetches a path under the site to a temp file, for a checker that
// reads a file rather than a URL. Fetched as Googlebot, because what matters
// is the rules Google is served — a site may answer differently.
func download(url string) (path string, cleanup func(), err error) {
	req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", func() {}, err
	}
	req.Header.Set("User-Agent", googlebot)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", func() {}, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", func() {}, err
	}
	if resp.StatusCode != http.StatusOK {
		// A 404 robots.txt is not an error: Google reads it as allow-all, so
		// the checker still has something true to say about an empty one.
		body = nil
	}
	f, err := os.CreateTemp("", "seo-*.txt")
	if err != nil {
		return "", func() {}, err
	}
	if _, err := f.Write(body); err != nil {
		return "", func() {}, err
	}
	_ = f.Close()
	return f.Name(), func() { _ = os.Remove(f.Name()) }, nil
}

// googlebot is what this asks as, since what matters is what Google is served.
const googlebot = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
