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

// Stop ends a program started with Start, and waits for it to go. Signalling
// the group rather than the process is what makes OwnGroup worth setting.
func Stop(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	_, _ = cmd.Process.Wait()
}
