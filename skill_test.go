package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The cli skill is prose about an API, and prose can drift. A verb's
// signature cannot — it is rendered from the flags — but nothing held the
// skill to the package it documents, and it went stale twice in a day: it
// showed a Verb without Desc, and told a reader to call CheckFlags after that
// function was deleted.
//
// So: every Go name the skill mentions must be one the package has. That is
// mechanical, and it is the check that would have caught both.
func TestSkillNamesOnlyWhatCLIHas(t *testing.T) {
	md, err := os.ReadFile("skill/cli.md")
	if err != nil {
		t.Fatal(err)
	}
	have := exported(t, "cli")
	for _, name := range mentioned(string(md)) {
		if !have[name] {
			t.Errorf("skill/cli.md names %q and package cli has no such thing; the skill is prose about an API, so it drifts unless something says so", name)
		}
	}
}

// mentioned is every Go name the skill's prose refers to: `Foo` or `cli.Foo`
// in inline code. A name needs a lowercase letter to count, which is what
// separates an identifier from a placeholder — Desc is one, DIR is not.
func mentioned(md string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile("`(?:cli\\.)?([A-Z][A-Za-z]*)`").FindAllStringSubmatch(md, -1) {
		name := m[1]
		if seen[name] || strings.ToUpper(name) == name {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// exported is every name a package offers a caller: its functions, its types,
// and the fields of its structs — because a skill says `Args` as readily as
// it says `Main`, and both are things the package either has or does not.
func exported(t *testing.T, dir string) map[string]bool {
	t.Helper()
	pkgs, err := parser.ParseDir(token.NewFileSet(), dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, pkg := range pkgs {
		ast.Inspect(pkg, func(n ast.Node) bool {
			switch d := n.(type) {
			case *ast.FuncDecl:
				have[d.Name.Name] = true
			case *ast.TypeSpec:
				have[d.Name.Name] = true
			case *ast.Field:
				for _, name := range d.Names {
					have[name.Name] = true
				}
			}
			return true
		})
	}
	return have
}
