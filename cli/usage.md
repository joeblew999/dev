### The tool itself

Every command built on this verb system gets these two without writing them.

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
