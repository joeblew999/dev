package main

import (
	"testing"

	"github.com/joeblew999/dev/cli"
)

// TestSkill holds every copy of the manual to the verbs; dev check runs it.
func TestSkill(t *testing.T) { cli.CheckSkill(t, dev) }
