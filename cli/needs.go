// What a command on this stack needs installed, and the mise line that
// installs it.
//
// dev shells out to fourteen programs and, until this existed, named the pin
// for exactly one of them. The rest failed with "not on PATH" and left the
// reader to work out what to add — which is fine once, on your own machine,
// and is the whole adoption cost for a repo that has just pinned dev and does
// not yet know what dev wants.
//
// One declaration serves both halves: the error a missing binary gives, and
// the list a repo needs before it hits that error. They cannot drift, because
// there is nothing to keep in step.
package cli

import (
	"sort"
	"strings"
)

// Need is one program, and how a repo gets it.
type Need struct {
	Bin string // what it is called on PATH
	// Key is what mise calls it, when that is not the binary's name. opentofu
	// ships tofu and node ships npm, so asking mise whether "tofu" is active
	// says no in a repo that pins opentofu and has it — which is the kind of
	// wrong answer that sends somebody to install what they already have.
	Key string
	// Pin is what mise is asked for — "flyctl@latest", "opentofu@latest",
	// "packslip:github.com/jdx/hk@2.0.1" — and "" when mise cannot install
	// it at all. The [tools] line is derived from this rather than written
	// beside it, because two spellings of one fact drift, and hand-writing
	// TOML is how `node = "latest"` was once added twice.
	Pin string
	For string // which of dev's verbs want it, so a repo can skip what it will never run
	// Why is what to say when mise cannot install it — Claude Code has its
	// own installer, and git is expected to be there already.
	Why string
}

// Line is the [tools] entry this would become, or Why when mise cannot
// install it. Derived from Pin so the two cannot disagree.
func (n Need) Line() string {
	if n.Pin == "" {
		return n.Why
	}
	name, version, ok := strings.Cut(n.Pin, "@")
	if !ok {
		return n.Pin
	}
	if strings.ContainsAny(name, ":/") {
		name = `"` + name + `"`
	}
	return name + ` = "` + version + `"`
}

// needs is every program, by the name it has on PATH.
//
// Kept here rather than beside each caller because the point of it is to be
// answerable as a set: "what does this repo need before it can use dev" is
// one question, and fourteen constants in nine packages cannot answer it.
var needs = map[string]Need{
	"go":         {Bin: "go", Pin: "go@1.27.1", For: "build, check, run, test"},
	"tinygo":     {Bin: "tinygo", Pin: "tinygo@latest", For: "wasm, for a Worker built from Go"},
	"node":       {Bin: "node", Pin: "node@latest", For: "wasm and deploy, because wrangler runs on it"},
	"npm":        {Bin: "npm", Key: "node", Pin: "node@latest", For: "a command directory holding a package.json"},
	"wrangler":   {Bin: "wrangler", Pin: "wrangler@latest", For: "deploy, delete, logs, smoke on Cloudflare"},
	"workerd":    {Bin: "workerd", Pin: "workerd@latest", For: "running a Worker locally"},
	"flyctl":     {Bin: "flyctl", Pin: "flyctl@latest", For: "deploy, delete, logs, list on Fly"},
	"fnox":       {Bin: "fnox", Pin: "fnox@latest", For: "every secret, and every cloud CLI runs under it"},
	"goreleaser": {Bin: "goreleaser", Pin: "goreleaser@latest", For: "release"},
	"packslip":   {Bin: "packslip", Pin: "packslip@latest", For: "release: the signed manifest mise installs from"},
	"gh":         {Bin: "gh", Pin: "gh@latest", For: "release: publishing it"},
	"go-mod-upgrade": {Bin: "go-mod-upgrade", Pin: "go:github.com/oligot/go-mod-upgrade@latest",
		For: "deps upgrade: choosing which modules to take"},
	"cloudflared": {Bin: "cloudflared", Pin: "cloudflared@latest",
		For: "fronting an app through a tunnel"},
	"tofu": {Bin: "tofu", Key: "opentofu", Pin: "opentofu@latest",
		For: "front and unfront: the Cloudflare zone changes, planned before they happen"},

	// The three mise does not install.
	"git": {Bin: "git", For: "which repository a directory is in",
		Why: "git is expected to be on the machine already"},
	"claude": {Bin: "claude", For: "session verify: holding a real session against the lock",
		Why: "install Claude Code: https://claude.com/product/claude-code"},
	"ps":   {Bin: "ps", For: "finding sessions that predate a sync", Why: "part of the operating system"},
	"lsof": {Bin: "lsof", For: "finding sessions that predate a sync", Why: "part of the operating system"},
}

// PinFor is the mise line that installs bin, so a missing binary names what
// to add rather than only what is absent. Empty for a program mise cannot
// install, which Cmd reports differently.
func PinFor(bin string) string { return needs[bin].Line() }

// Needs is every program a command on this stack may run, in name order.
func Needs() []Need {
	out := make([]Need, 0, len(needs))
	for _, n := range needs {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bin < out[j].Bin })
	return out
}

// Installable reports whether mise can fetch this one. git, ps and lsof are
// needed and cannot be installed, and an error that offers a mise line for
// them sends the reader somewhere that will not help.
func Installable(bin string) bool { return needs[bin].Pin != "" }

// Spec is what `mise use` is given.
func (n Need) Spec() string { return n.Pin }

// MiseKey is what mise calls this tool, which is its binary's name unless it
// says otherwise.
func (n Need) MiseKey() string {
	if n.Key != "" {
		return n.Key
	}
	return n.Bin
}

// Register adds programs to the registry, for a package that runs one cli
// has never heard of.
//
// The registry above is what cli itself shells out to. `dev seo` runs seven
// more, and they were declared a second time in their own package — with the
// [tools] line written out by hand, which is the one thing Need.Line exists
// to stop. So `dev tools` listed fourteen programs while the binary ran
// twenty-one, and a repo adopting `dev seo` met them one missing binary at a
// time: exactly the adoption cost this file was written to end.
//
// Called from a package's init, so by the time anything asks the registry is
// whole. Declaring the same binary twice with different pins is a programming
// error and says so, rather than letting whichever package initialised last
// decide what version a repo installs.
func Register(more ...Need) {
	for _, n := range more {
		if was, ok := needs[n.Bin]; ok && was != n {
			panic("cli: " + n.Bin + " is declared twice and differently: " +
				was.Pin + " (for " + was.For + ") and " + n.Pin + " (for " + n.For + ")")
		}
		needs[n.Bin] = n
	}
}
