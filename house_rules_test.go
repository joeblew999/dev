// Two of AGENTS.md's rules, made mechanical.
//
// Both of these are ordinarily a linter's job, and neither can be one here:
// forbidigo and depguard only exist inside golangci-lint, which this tree
// measured and rejected, and standalone they both die on this toolchain with
// `internal error: package "os" without types`. A rule that cannot run is a
// comment, so they are written here instead — go/ast and go list are in the
// standard library, they cost go.mod nothing, and `mise run check` already
// runs every test.
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The two libraries dev must never import. AGENTS.md's "Work goes through the
// cloud's CLI" section is the argument in full, with the numbers behind it —
// the short of it is that flyctl and wrangler are binaries the registry
// fetches, while these are code dev would become, and the warm rebuild that
// every edit here waits on doubles when they are linked in.
//
// Neither is in go.mod today, so this cannot fail by accident. What it stops
// is the afternoon somebody reaches for an SDK to save an hour of naming
// JSON fields correctly, and nothing says the cost until the next cold build.
var forbiddenModules = []string{
	"github.com/cloudflare/cloudflare-go",
	"github.com/superfly/fly-go",
}

func TestNothingImportsACloudSDK(t *testing.T) {
	// go list -deps is the whole import graph, not just what this file's
	// package reaches, so a dependency three packages down is still caught.
	out, err := exec.Command("go", "list", "-deps", "./...").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps ./...: %v\n%s", err, out)
	}
	for pkg := range strings.FieldsSeq(string(out)) {
		for _, banned := range forbiddenModules {
			if pkg == banned || strings.HasPrefix(pkg, banned+"/") {
				t.Errorf("%s is in the import graph, via %s.\n"+
					"AGENTS.md measured this one: a cloud SDK is a library dev becomes, not a "+
					"binary dev runs, and it is paid for on every rebuild. The CLI does the work; "+
					"what the CLI cannot answer is hand-rolled HTTP in internal/cloudflare.", banned, pkg)
			}
		}
	}
}

// TestOnlyTheCallsStdoutIsWrittenTo fails on fmt.Print, fmt.Printf and
// fmt.Println outside tests.
//
// Every verb is handed the Call's Stdout and Stderr, which is what lets a
// test read what a verb said and a caller send it somewhere. A bare
// fmt.Printf goes to the process's own stdout instead, around all of that,
// and there is no way to tell from the call site that it has. Exactly one
// lived here — the line `dev release` prints when a release is published,
// which is that verb's entire answer and the one thing a test of it would
// want to read.
//
// fmt.Fprint* is fine and is how the rest of the tree writes: the point is
// not that output is rare, it is that output goes somewhere a caller chose.
func TestOnlyTheCallsStdoutIsWrittenTo(t *testing.T) {
	banned := map[string]bool{"Print": true, "Printf": true, "Println": true}
	for _, path := range goFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			// fmt.Printf is a selector on a bare identifier, so this reads
			// the shape rather than resolving types: a local variable called
			// fmt would fool it, and a file with one has a worse problem.
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "fmt" || !banned[sel.Sel.Name] {
				return true
			}
			t.Errorf("%s: fmt.%s writes to the process's stdout, around the Call the verb was handed.\n"+
				"Write to the Call's Stdout instead — fmt.Fprintf(c.Stdout, ...) — so a test can read it "+
				"and a caller can redirect it. Progress and warnings go to c.Stderr.",
				fset.Position(call.Pos()), sel.Sel.Name)
			return true
		})
	}
}

// goFiles is every non-test .go file in the repo, which is what both rules
// are about: a test printing to stdout is printing to the test's own output,
// and go test owns that.
func goFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(".", func(p string, d os.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case d.IsDir():
			// The same skips stale_sources_test.go makes, for the same
			// reason: build output and git's own objects are not source.
			if name := d.Name(); name == ".git" || name == ".bin" || name == ".dist" {
				return filepath.SkipDir
			}
			return nil
		case filepath.Ext(p) != ".go" || strings.HasSuffix(p, "_test.go"):
			return nil
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
