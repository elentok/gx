package ralphloop

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// parkStatuses are the statuses only the park path may write.
var parkStatuses = map[string]bool{"StatusNeedsAnswer": true, "StatusNeedsRepair": true}

// parkWriters are the claim.go helpers that write a park status; only park.go
// may call them.
var parkWriters = map[string]bool{
	"markNeedsAnswer":                  true,
	"markNeedsAnswerWithReasonAndStub": true,
	"markNeedsRepairWithReason":        true,
}

// TestParkPath_NoStatusWriteBypasses is Seam F: it fails when non-test code
// writes a needs-answer/needs-repair status anywhere but claim.go's park
// writers, or calls those writers from anywhere but park.go — so a future
// catch-all cannot park a ticket without the park path's event and notification.
func TestParkPath_NoStatusWriteBypasses(t *testing.T) {
	root := filepath.Join("..")
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		inRalphloop := filepath.Clean(filepath.Dir(path)) == filepath.Join("..", "ralphloop")
		base := filepath.Base(path)
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if isSetStatusToPark(n) {
					t.Errorf("%s: SetStatus writes a park status outside the park path", fset.Position(n.Pos()))
				}
				if id, ok := n.Fun.(*ast.Ident); ok && parkWriters[id.Name] && !(inRalphloop && (base == "park.go" || base == "claim.go")) {
					t.Errorf("%s: calls %s outside the park path", fset.Position(n.Pos()), id.Name)
				}
			case *ast.AssignStmt:
				for _, rhs := range n.Rhs {
					if isParkStatus(rhs) && !(inRalphloop && base == "claim.go") {
						t.Errorf("%s: assigns a park status outside claim.go's park writers", fset.Position(n.Pos()))
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func isParkStatus(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && parkStatuses[sel.Sel.Name]
}

// isSetStatusToPark catches the generic helper being used to park, e.g.
// SetStatus(path, "needs-repair").
func isSetStatusToPark(c *ast.CallExpr) bool {
	var name string
	switch fn := c.Fun.(type) {
	case *ast.Ident:
		name = fn.Name
	case *ast.SelectorExpr:
		name = fn.Sel.Name
	}
	if name != "SetStatus" || len(c.Args) < 2 {
		return false
	}
	lit, ok := c.Args[1].(*ast.BasicLit)
	return ok && (lit.Value == `"needs-answer"` || lit.Value == `"needs-repair"`)
}
