package main

import (
	"testing"

	"github.com/joeblew999/dev/cli"
)

// TestSkill holds every copy of the manual to the verbs; dev check runs it.
func TestSkill(t *testing.T) { cli.CheckSkill(t, dev) }

// TestUsage holds every verb's usage to the markdown subset the terminal
// rendering can read, and catches a `<placeholder>` written without the
// backticks that stop a renderer eating it.
func TestUsage(t *testing.T) { cli.CheckUsage(t, dev) }
