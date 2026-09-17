// What a verb declares, and the signature rendered from it. A verb says what
// only it knows — its positionals, its flags, its one line — and everything a
// manual shows about it is built from that, so the two cannot disagree.
package cli

import (
	_ "embed"
	"flag"
	"io"
	"strings"
)

// Verb is one verb of a command: what runs, what it takes, and what it is
// for.
//
// A verb's signature is rendered from Args and Flags, never written: the
// flags are registered in Go already, so typing them into a manual too is a
// second copy of one fact, and the copies drift. `dev release --rotate` was
// registered and named in no manual at all until someone read both by hand.
// Args and Subs are written because nothing else knows them — a positional
// and a subcommand are declared nowhere in a FlagSet.
//
// Usage is therefore the description and nothing else: what the verb does,
// which no code can tell you.
type Verb struct {
	Run   Runner
	Args  string              // the positionals: "DIR", "URL", "DIR [VERSION]"
	Flags func(*flag.FlagSet) // registers them; Run calls it too, so there is one registration
	Desc  string              // one line: what this verb is for, next to the flags it takes
	Usage string              // the group's prose: why these verbs exist, what they share
	Subs  map[string]Verb     // secrets set, deps list: each with its own Args and Flags
}

// Signature is how a verb is written in a manual and in help: the command, the
// verb, its positionals, then every flag it registers, in name order.
//
// This is check I7 of the i18n plan made real — "the rendered skill carries
// every flag in the verb table" — by rendering from the registration rather
// than from prose beside it.
func (v Verb) Signature(name, verb string) string {
	var b strings.Builder
	b.WriteString(name + " " + verb)
	if v.Args != "" {
		b.WriteString(" " + v.Args)
	}
	for _, f := range v.flagSpecs() {
		b.WriteString(" " + f)
	}
	return b.String()
}

// flagSpecs is each registered flag as a manual writes it — [--name VALUE] —
// in name order, which is the order a FlagSet visits them.
func (v Verb) flagSpecs() []string {
	if v.Flags == nil {
		return nil
	}
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	v.Flags(fs)
	var out []string
	fs.VisitAll(func(f *flag.Flag) {
		out = append(out, "[--"+f.Name+placeholder(f)+"]")
	})
	return out
}

// placeholder is what a flag takes, or "" when it takes nothing. A bool is
// written bare, because writing [--yes BOOL] would invite someone to type it.
//
// The name comes from the flag's own help text, through the standard
// library's convention: a backquoted word there names the value, and
// PrintDefaults strips the quotes. So `--env` reading "wrangler `NAME`"
// prints as [--env NAME] here and as "-env NAME" under --help, from one
// string. Without backquotes the type is used, which is what the standard
// library does too.
func placeholder(f *flag.Flag) string {
	if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
		return ""
	}
	name, _ := flag.UnquoteUsage(f)
	if name == "" {
		return ""
	}
	return " " + name
}
