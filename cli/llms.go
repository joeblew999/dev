// The command described for a language model: llms.txt, in the shape
// llmstxt.org asks for, rendered from the same verb table the manual and the
// terminal index are rendered from.
//
// It belongs to cli rather than to whatever writes a site's other files
// because a command's verbs are the one thing only the command knows. A tool
// that writes llms.txt from a crawl can say which pages exist and nothing
// about what the binary does, and it would describe itself rather than the
// command being documented if it tried — so every command on the stack gets
// `<cmd> llms` and answers for itself.
package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
)

// LLMsFile is what the convention calls the file. It is read from the root of
// a site and nowhere else, which is why the verb takes the directory that is
// the site rather than a path.
const LLMsFile = "llms.txt"

// LLMsDoc is a document in llmstxt.org's shape: an H1 naming the thing, one
// line of summary as a blockquote, then H2 sections of links, each with what
// it is for after the link.
//
// The shape lives here rather than at each place that writes one, because a
// site's llms.txt and a command's are the same document about different
// subjects — and two spellings of one format drift the day either is edited.
type LLMsDoc struct {
	Title    string
	Summary  string
	Sections []LLMsSection
}

// LLMsSection is one H2 and the links under it. One with no links is not
// written at all: a heading over nothing tells a reader there is something
// they are not being shown.
type LLMsSection struct {
	Name  string
	Links []LLMsLink
}

// LLMsLink is one entry: what to call it, where it is, and what it is for.
// Note is what a model reads to decide whether to follow the link, so it is
// the line the thing already says about itself and never a second wording of
// it.
type LLMsLink struct{ Text, URL, Note string }

// String is the document as the file holds it.
func (d LLMsDoc) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", d.Title)
	if d.Summary != "" {
		fmt.Fprintf(&b, "\n> %s\n", d.Summary)
	}
	for _, s := range d.Sections {
		if len(s.Links) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n", s.Name)
		for _, l := range s.Links {
			fmt.Fprintf(&b, "- [%s](%s)", l.Text, l.URL)
			if l.Note != "" {
				fmt.Fprintf(&b, ": %s", l.Note)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// Links is how many entries the document carries, so a caller can say what it
// wrote without counting the lines back out of the text.
func (d LLMsDoc) Links() int {
	n := 0
	for _, s := range d.Sections {
		n += len(s.Links)
	}
	return n
}

// LLMs is this command as llms.txt: every verb under the heading of the group
// it belongs to, each with the one line it says about itself and a link to the
// manual that explains it, then the manuals themselves.
//
// Nothing here is a list of verbs. It reads all() and groups(), which is what
// the manual and the terminal index read, so the three cannot disagree about
// what this command answers to — and CheckSurfaces fails the build when they
// do.
//
// origin is the site the manual is published on. Without one the links are the
// repo-relative paths `skill` writes, which is what a reader has to go on
// before anything is published.
func (c Command) LLMs(origin string) LLMsDoc {
	verbs := c.all()
	manual := manualURL(origin, c.Name)
	doc := LLMsDoc{Title: c.Name, Summary: frontmatter(c.Skill, "description")}
	for _, g := range c.groups() {
		paths := expand(verbs, g.paths)
		if len(paths) == 0 {
			continue
		}
		// The group's own heading, so a section is called what the manual
		// calls it. A group whose prose has none is named after the first
		// verb in it, which at least tells two such sections apart.
		section := LLMsSection{Name: Or(heading(g.prose), paths[0])}
		for _, path := range paths {
			v, _, ok := lookup(verbs, path)
			if !ok {
				continue
			}
			section.Links = append(section.Links, LLMsLink{
				Text: v.Signature(c.Name, path), URL: manual, Note: v.Desc})
		}
		doc.Sections = append(doc.Sections, section)
	}
	// Where the prose is, named rather than left to be inferred from a link
	// repeated above: a model that wants the whole of it should be told which
	// file that is, and a skill this command ships is a document in its own
	// right that no verb above points at.
	manuals := LLMsSection{Name: "Manuals", Links: []LLMsLink{{
		Text: c.Name, URL: manual,
		Note: "every verb above in full: what it takes, and the prose around it"}}}
	for _, name := range SortedKeys(c.Skills) {
		manuals.Links = append(manuals.Links, LLMsLink{
			Text: name, URL: manualURL(origin, name),
			Note: frontmatter(c.Skills[name], "description")})
	}
	doc.Sections = append(doc.Sections, manuals)
	return doc
}

// manualURL is where a manual is read: the path `skill` writes it to, made
// absolute when the caller named the site it is published on. One place, so an
// llms.txt cannot point somewhere the manual is not. Joined with slashes and
// never filepath.Join, because it is a URL on every platform.
func manualURL(origin, name string) string {
	path := ShippedDir + "/" + name + "/" + SkillFile
	if origin == "" {
		return path
	}
	return strings.TrimSuffix(origin, "/") + "/" + path
}

// heading is a group's own heading — the first one its prose writes — which
// is what the manual calls that section. Reading it back is what stops the
// llms.txt inventing a second name for a section that already has one.
func heading(prose string) string {
	for _, line := range Lines(prose) {
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "# "))
		}
	}
	return ""
}

// frontmatter is one key's value out of a markdown file's leading --- block,
// which is where a skill already says in one line what it is for. Reading it
// back is what stops a command carrying a second description of itself that
// somebody has to remember to keep in step.
func frontmatter(md, key string) string {
	lines := Lines(md)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			return ""
		}
		// Cut at the first colon only: what a description says about the
		// command has colons of its own, and they belong to the value.
		if name, value, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(name) == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// llms is `<cmd> llms [DIR]`: the document to stdout, or written as llms.txt
// into the directory that is the site.
//
// Stdout is the default because the document is data — a build pipes it, a
// person reads it, and neither wants a file left behind. A directory is taken
// because the file has one right place, and asking every repo to remember
// that its name is llms.txt and that it goes at the root is asking for it to
// be got wrong.
func (c Command) llms(call Call) error {
	doc := c.LLMs(call.Value("origin"))
	if len(call.Args) == 0 {
		fmt.Fprint(call.Stdout, doc)
		return nil
	}
	path := filepath.Join(call.Args[0], LLMsFile)
	if _, err := put(call.Stderr, c.Name, path, doc.String(), false); err != nil {
		return err
	}
	// To stderr, because stdout carries the document when there is no
	// directory and must carry nothing else when there is.
	fmt.Fprintf(call.Stderr, "wrote %s: %s\n", rel(path), Plural(doc.Links(), "link"))
	return nil
}

// llmsFlags is `llms --origin URL`.
func llmsFlags(fs *flag.FlagSet) {
	fs.String("origin", "", "the site the manual is published on, as a `URL`, so the links are absolute")
}
