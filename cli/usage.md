### The tool itself

Every command built on this verb system gets these without writing them.

### Describing the command to a model

`llms` is this command as an llms.txt, the file llmstxt.org asks a site to
serve at its root: the command's name, the one line its manual's frontmatter
already says about it, then every verb under the heading of the group it
belongs to, each with what it is for and a link to the manual that explains
it. It is rendered from the verb table, the same one `skill` renders and the
same one the index prints, so a verb cannot reach one of them and not the
others.

`--origin` is the site the manual is published on, which makes those links
absolute; without it they are the repo-relative paths `skill` writes, which is
what a reader has to go on before anything is published. A directory writes
the file into it, because the convention reads it from the root of a site and
nowhere else; with no directory it goes to stdout, where a build can pipe it.

Write this rather than an llms.txt generated from a list of pages when the
site documents a command. The two answer different questions — what the
command does, against what pages exist — and only the command can answer the
first about itself.

### Tools

`tools` is every program this command may run, what each is for, and whether
this directory has it. A repo that has just pinned this command knows one
line and nothing else, and would otherwise learn the rest one failure at a
time.

`--add` has mise record what is missing, leaving alone the versions the repo
already chose. `--fresh` replaces what is pinned with exactly what this
command needs, and makes a config when there is none — which is what a
scratch directory wants, where the question is not what is missing but what a
working one looks like. It says what it is about to replace and asks first.

Both go through `mise use` rather than editing a config, because mise owns
that file: it knows which spelling this directory uses, it spells a backend
correctly, and it records things a hand-written line would silently drop —
packslip's signing identity, for one. It is given an explicit path, so it can
only ever write this directory's own config and never the machine's.

mise itself is never installed. It is the thing that installs things, and
that is not a side effect this should have; where to get it is all that is
appropriate.
