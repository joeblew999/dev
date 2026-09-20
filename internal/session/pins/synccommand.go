package pins

import (
	"sync"

	"github.com/joeblew999/dev/internal/conf"
)

// How this repo spells "run a sync", quoted back by every message that tells a
// reader to run one.
//
// It used to be a package variable that two different functions assigned, one
// of them a wrapper every subcommand had to remember to be wrapped in. What an
// error said therefore depended on whether something earlier had run, and a
// new subcommand that forgot the wrapper printed a command that repo does not
// have — with nothing to notice.
//
// It is a fact of the repo, so it is read from the repo, once, when something
// first needs it. A verb cannot be in the wrong order with respect to a fact.
var syncCommand = sync.OnceValue(readSyncCommand)

// readSyncCommand is the read itself, kept apart from the memo so a test can
// run it against a session.toml it just wrote. Memoised state is the right
// shape for a fact about the repo and the wrong shape for a test, and the
// answer to that is to test the reading rather than to make the fact
// resettable — which would put the hazard back.
func readSyncCommand() string {
	const binary = "dev session sync"
	config, err := conf.Load[struct {
		SyncCommand string `toml:"sync_command"`
	}](File)
	if err != nil || config.SyncCommand == "" {
		return binary
	}
	return config.SyncCommand
}

// SyncCommand is how a message tells this repo's reader to run a sync.
func SyncCommand() string { return syncCommand() }

// VerifyCommand is the sibling verb, spelled the same way: a repo that runs
// sync as `mise run session:sync` runs verify as `mise run session:verify`.
func VerifyCommand() string { return SwapVerb(SyncCommand(), "sync", "verify") }

// SwapVerb replaces a trailing verb, leaving the rest of the spelling alone,
// so advice always names a command spelled the way this repo spells them.
func SwapVerb(cmd, from, to string) string {
	if len(cmd) >= len(from) && cmd[len(cmd)-len(from):] == from {
		return cmd[:len(cmd)-len(from)] + to
	}
	return cmd + " " + to
}
