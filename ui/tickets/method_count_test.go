package tickets

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// methodCounts counts the methods declared on each receiver type across the
// package's non-test files.
func methodCounts(t *testing.T) map[string]int {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			if id, ok := recv.(*ast.Ident); ok {
				counts[id.Name]++
			}
		}
	}
	return counts
}

func TestModelSplitMethodCounts(t *testing.T) {
	counts := methodCounts(t)
	limits := map[string]int{"Model": 50}
	for typ, limit := range limits {
		if got := counts[typ]; got == 0 || got > limit {
			t.Errorf("%s has %d methods, want 1..%d", typ, got, limit)
		}
	}
}
