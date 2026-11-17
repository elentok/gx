// Package setstateguard holds the repo-wide guard against calling the runner
// fake's SetState in a cleanup. SetState panics with "no live session" when
// the launch never made the session, and that panic ends the test binary. A
// cleanup uses the fake's safe Finish helper instead.
//
// The scan sees syntax only, so it cannot know the receiver type: it matches
// the method name SetState. If another type grows a SetState that a cleanup
// needs, rename that method; there is no exemption.
package setstateguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/repofiles"
)

// selfFile holds sample sources that contain the pattern on purpose.
const selfFile = "setstateguard/setstateguard_test.go"

const message = "use the runner fake's safe finish helper in a cleanup; SetState panics for a missing session"

func isCleanupCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Cleanup"
}

// scan returns one "path:line" per SetState call inside a function literal
// passed to Cleanup, or inside a defer.
func scan(path, src string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}
	seen := map[token.Pos]bool{}
	var found []string
	collect := func(root ast.Node) {
		ast.Inspect(root, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetState" && !seen[call.Pos()] {
				seen[call.Pos()] = true
				found = append(found, fmt.Sprintf("%s:%d", path, fset.Position(call.Pos()).Line))
			}
			return true
		})
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.DeferStmt:
			collect(s.Call)
		case *ast.CallExpr:
			if isCleanupCall(s) {
				for _, arg := range s.Args {
					if lit, ok := arg.(*ast.FuncLit); ok {
						collect(lit)
					}
				}
			}
		}
		return true
	})
	sort.Strings(found)
	return found, nil
}

func goFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, p := range repofiles.List(t, "*_test.go") {
		if p != selfFile {
			files = append(files, p)
		}
	}
	for _, p := range repofiles.List(t, "server/servertest/*.go") {
		if !strings.HasSuffix(p, "_test.go") {
			files = append(files, p)
		}
	}
	return files
}

func TestNoRawSetStateCleanup(t *testing.T) {
	var problems []string
	for _, path := range goFiles(t) {
		src, err := os.ReadFile(filepath.Join("..", path))
		if os.IsNotExist(err) {
			continue // tracked but deleted in the working tree
		}
		if err != nil {
			t.Fatal(err)
		}
		found, err := scan(path, string(src))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		problems = append(problems, found...)
	}
	for _, p := range problems {
		t.Errorf("%s: %s", p, message)
	}
}

func TestScan(t *testing.T) {
	cases := []struct {
		name, body string
		want       int
	}{
		{"raw cleanup", `t.Cleanup(func() { fake.SetState("x", agentrunner.StateDone, "") })`, 1},
		{"b cleanup", `b.Cleanup(func() { fake.SetState("x", s, "") })`, 1},
		{"defer closure", `defer func() { fake.SetState("x", s, "") }()`, 1},
		{"defer direct", `defer fake.SetState("x", s, "")`, 1},
		{"guarded cleanup still found", `t.Cleanup(func() { if len(fake.Prompts("x")) > 0 { fake.SetState("x", s, "") } })`, 1},
		{"test body", `fake.SetState("x", s, "")`, 0},
		{"cleanup with helper", `t.Cleanup(func() { fake.Finish("x") })`, 0},
		{"two calls", `t.Cleanup(func() { fake.SetState("a", s, ""); fake.SetState("b", s, "") })`, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "package p\nfunc f() {\n" + c.body + "\n}\n"
			got, err := scan("p_test.go", src)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != c.want {
				t.Errorf("found %v, want %d", got, c.want)
			}
		})
	}
}
