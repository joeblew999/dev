//go:build !unix

package tool

import "os/exec"

// OwnGroup is what a process group costs where there are none: nothing.
func OwnGroup(*exec.Cmd) {}

// Stop ends a program started with Start, and waits for it to go.
func Stop(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}
