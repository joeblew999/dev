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

// parse turns what was typed into a Call, using what the verb declared. Args
// beginning with DIR means the first argument is a directory; the rest are
// positionals, and everything after a bare -- is passed through verbatim.
//
// A verb that wants none of this still gets it: with no Args and no Flags the
// arguments arrive untouched, which is what a subcommand like `secrets ci`
// needs.
func (v Verb) parse(verb string, args []string, stdout, stderr io.Writer) (Call, error) {
	fs := Flags(verb, stderr)
	if v.Flags != nil {
		v.Flags(fs)
	}
	c := Call{Verb: verb, Flags: fs, Stdin: Stdin, Stdout: stdout, Stderr: stderr}
	if strings.HasPrefix(v.Args, "DIR") {
		dir, rest, err := DirAnd(fs, args, -1)
		if err != nil {
			return c, err
		}
		c.Dir, c.Args = dir, rest
		return c, v.checkArgs(verb, rest, 1)
	}
	if HelpRequested(args) {
		fs.Usage()
		return c, ErrHelp
	}
	rest, err := ParseInterleaved(fs, args)
	if err != nil {
		return c, Usagef("%s: %v", verb, err)
	}
	// Args is the one place that knows what a verb takes, so cli holds a call
	// to it rather than each verb opening with its own length check. Four
	// verbs had written one, in four wordings, and every other verb that
	// should have had one silently ignored whatever it was handed.
	if err := v.checkArgs(verb, rest, 0); err != nil {
		return c, err
	}
	c.Args = rest
	return c, nil
}

// checkArgs holds a call to what Args declares. A bare word is required, a
// [bracketed] one optional, and one ending in ... takes the rest — the shapes
// a manual already used, now read rather than only printed.
func (v Verb) checkArgs(verb string, rest []string, skip int) error {
	need, most, variadic := v.arity(skip)
	switch {
	case len(rest) < need:
		return Usagef("%s: needs %s", verb, v.Args)
	case !variadic && len(rest) > most && v.Args == "":
		return Usagef("%s: takes no arguments", verb)
	case !variadic && len(rest) > most:
		return Usagef("%s: takes %s", verb, v.Args)
	}
	return nil
}

// arity reads Args: how many positionals are required, how many are accepted,
// and whether the last one swallows the rest.
// Skip is how many leading words are already accounted for: one for a verb
// whose Args begin with DIR, since cli pulled the directory out before this.
func (v Verb) arity(skip int) (need, most int, variadic bool) {
	words := strings.Fields(v.Args)
	if skip < len(words) {
		words = words[skip:]
	} else {
		words = nil
	}
	for _, word := range words {
		switch {
		case word == "--" || strings.HasPrefix(word, "[--"):
			// Everything after a bare -- belongs to the program being run.
			return need, most, true
		case strings.HasSuffix(word, "..."):
			// NAME... is one or more; [SOURCE...] is none or more.
			if !strings.HasPrefix(word, "[") {
				need++
			}
			return need, most, true
		case strings.HasPrefix(word, "["):
			most++
		default:
			need++
			most++
		}
	}
	return need, most, variadic
}
