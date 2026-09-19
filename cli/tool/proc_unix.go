//go:build unix

package tool

import (
	"os/exec"
	"syscall"
)

// OwnGroup puts the program in a process group of its own, so stopping it
// stops what it spawned. wrangler dev starts a child; killing only the parent
// leaves the port held and the next run fails for a reason nobody can see.
func OwnGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// end signals one running program. Signalling the group rather than the
// process is what makes OwnGroup worth setting.
func end(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
