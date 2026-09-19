package main

import (
	"testing"

	"github.com/joeblew999/dev/cli/skillcheck"
)

// The cli skill explains a library this repo asks other repos to build on, so
// it has to be right about it. Nothing held it to the package until now, and
// it went stale twice in one day.
//
// cli/tool is listed because the skill documents it: a package the prose
// names has to be a package the check reads, or the prose drifts there
// instead.
func TestCLISkillNamesOnlyWhatExists(t *testing.T) {
	skillcheck.Names(t, cliSkill, "cli", "cli/skillcheck", "cli/tool")
}
