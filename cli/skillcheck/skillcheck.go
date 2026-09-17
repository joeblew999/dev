// Package skillcheck holds a skill's prose to the package it documents.
//
// A verb's signature cannot drift, because cli renders it from the flags the
// verb registers. A skill that explains a library is prose, and prose drifts:
// this repo's own went stale twice in one day, showing a struct field that
// had been added and naming a function that had been deleted, and both times
// a person found it by reading the two side by side.
//
// It is a package of its own rather than part of cli because it parses Go,
// and go/parser is 66 packages. cli is linked into every command built on it,
// including a Worker's wasm, so what a test needs does not belong there.
package skillcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
)

// TB is the part of testing.TB this needs, so a caller's binary never links
// the testing package.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
}

// Names fails when the markdown names a Go identifier that none of dirs has.
// A repo writing a skill about its own package calls it with that skill and
// that package:
//
//	skillcheck.Names(t, skillMarkdown, "internal/thing")
//
// dirs are package directories relative to the test's own, and all of them
// are searched, so a skill covering more than one package names one per
// directory.
func Names(t TB, md string, dirs ...string) {
	t.Helper()
	have := map[string]bool{}
	for _, dir := range dirs {
		for name := range exported(t, dir) {
			have[name] = true
		}
	}
	for _, name := range Mentioned(md) {
		if !have[name] {
			t.Errorf("the skill names %q and no package here has it; a skill is prose about an API, so it drifts unless something says so", name)
		}
	}
}

// Mentioned is every Go name a skill's prose refers to: `Foo` or `pkg.Foo` in
// inline code.
//
// A name needs a lowercase letter in it to count. That is what separates an
// identifier from a placeholder — `Desc` is one, `DIR` and `TEXT` are not —
// and it needs no list of exceptions to keep current.
func Mentioned(md string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile("`(?:[a-z][a-z0-9]*\\.)?([A-Z][A-Za-z0-9]*)`").FindAllStringSubmatch(md, -1) {
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
// and its structs' fields, because a skill says a field as readily as a
// function and both are things the package either has or does not.
func exported(t TB, dir string) map[string]bool {
	pkgs, err := parser.ParseDir(token.NewFileSet(), dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Errorf("reading %s: %v", dir, err)
		return nil
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
