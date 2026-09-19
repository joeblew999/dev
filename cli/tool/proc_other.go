//go:build !unix

package tool

import "os/exec"

// OwnGroup is what a process group costs where there are none: nothing.
func OwnGroup(*exec.Cmd) {}

// end signals one running program, where there are no process groups.
func end(cmd *exec.Cmd) { _ = cmd.Process.Kill() }
