package session

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/joeblew999/dev/cli/tool"
	"github.com/joeblew999/dev/internal/session/vendored"

	"github.com/joeblew999/dev/cli"
)

// A session that started before these skills changed may be holding a stale
// view of them, and saying so beats leaving someone to wonder why the agent
// ignores a skill that is right there.
//
// It used to say the session "cannot see them" and to restart. That is not
// true and was never checked: editing a SKILL.md applies without a restart,
// which Claude Code documents, and a skill directory that appears while a
// session runs was picked up here without one either.
//
// What is not documented either way is removal, which is the half that
// matters after `remove`: the files are gone and the session may still be
// offering them. So the advice is /reload-plugins — documented to reload
// skills along with plugins, hooks and agents, and far cheaper than losing a
// session — with a restart named only as the fallback it is.

type session struct {
	pid     int
	started time.Time
}

func warnStaleSessions(out io.Writer, now time.Time) {
	newest, ok := newestModTime(vendored.Primary())
	if !ok {
		return
	}
	for _, s := range claudeSessions(now) {
		if s.started.Before(newest) {
			fmt.Fprintf(out, "\nClaude Code (pid %d, started %s) is older than these skills (%s),\n",
				s.pid, s.started.Format("Jan 2 15:04"), newest.Format("Jan 2 15:04"))
			fmt.Fprintln(out, "so it may still be offering a skill that has changed or gone. Run")
			fmt.Fprintln(out, "/reload-plugins in that session; it reloads skills without losing it.")
			fmt.Fprintln(out, "(Claude Code before 2.1.260 has no such command: restart instead.)")
		}
	}
}

func newestModTime(dir string) (time.Time, bool) {
	var newest time.Time
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest, err == nil && !newest.IsZero()
}

// claudeSessions lists Claude Code processes whose working directory is this
// repo. It shells out to ps, which both macOS and Linux have; anything it cannot
// work out is skipped, because this is only a warning.
func claudeSessions(now time.Time) []session {
	res, err := tool.Cmd{Bin: PsBin, Args: []string{"-eo", "pid=,etime=,command="}, Quiet: true}.Capture()
	out := res.Out
	if err != nil {
		return nil
	}
	repo, err := os.Getwd()
	if err != nil {
		return nil
	}

	var sessions []session
	for _, line := range cli.Lines(out) {
		fields := strings.Fields(line)
		if len(fields) < 3 || !isClaudeBinary(fields[2]) {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		elapsed, err := parseElapsed(fields[1])
		if err != nil || processCwd(pid) != repo {
			continue
		}
		sessions = append(sessions, session{pid: pid, started: now.Add(-elapsed)})
	}
	return sessions
}

// isClaudeBinary matches the Claude Code binary, whether it comes from the CLI
// install or the VS Code extension, and not other commands mentioning claude.
func isClaudeBinary(command string) bool {
	base := filepath.Base(command)
	return (base == "claude" || base == "claude.exe") && strings.Contains(command, "/")
}

// parseElapsed reads ps etime ([[dd-]hh:]mm:ss). ps prints start times in the
// machine's locale, so elapsed time is the portable way to date a process.
func parseElapsed(etime string) (time.Duration, error) {
	etime = strings.TrimSpace(etime)
	var days int
	if before, after, found := strings.Cut(etime, "-"); found {
		d, err := strconv.Atoi(before)
		if err != nil {
			return 0, fmt.Errorf("elapsed %q: %w", etime, err)
		}
		days, etime = d, after
	}
	parts := strings.Split(etime, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("elapsed %q: want [[dd-]hh:]mm:ss", etime)
	}
	seconds := 0
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return 0, fmt.Errorf("elapsed %q: %w", etime, err)
		}
		seconds = seconds*60 + n
	}
	return time.Duration(days)*24*time.Hour + time.Duration(seconds)*time.Second, nil
}

func processCwd(pid int) string {
	if runtime.GOOS == "linux" {
		if dir, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid)); err == nil {
			return dir
		}
		return ""
	}
	res, err := tool.Cmd{Bin: LsofBin, Args: []string{"-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn"}, Quiet: true}.Capture()
	out := res.Out
	if err != nil {
		return ""
	}
	for _, line := range cli.Lines(out) {
		if after, ok := strings.CutPrefix(line, "n"); ok {
			return after
		}
	}
	return ""
}
