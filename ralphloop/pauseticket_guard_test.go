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

// pauseStatuses are the statuses only the pause path may write.
var pauseStatuses = map[string]bool{"StatusPaused": true, "StatusLegacyAnswer": true, "StatusLegacyRepair": true}

// pauseWriters are the claim.go helpers that write a pause status; only pause.go
// may call them.
var pauseWriters = map[string]bool{
	"markAsking":           true,
	"markRepairWithReason": true,
}

// TestPausePath_NoStatusWriteBypasses is Seam F: it fails when non-test code
// writes a paused (answer)/paused (repair) status anywhere but claim.go's pause
// writers, or calls those writers from anywhere but pauseticket.go — so a future
// catch-all cannot pause a ticket without the pause path's event and notification.
func TestPausePath_NoStatusWriteBypasses(t *testing.T) {
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
		// The one-off ticket migration rewrites legacy statuses to paused; it is
		// not a runtime pause, so it owes no event or notification.
		if filepath.ToSlash(filepath.Clean(path)) == "../tickets/migrate_pause.go" {
			return nil
		}
		inRalphloop := filepath.Clean(filepath.Dir(path)) == filepath.Join("..", "ralphloop")
		base := filepath.Base(path)
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if isSetStatusToPause(n) {
					t.Errorf("%s: SetStatus writes a pause status outside the pause path", fset.Position(n.Pos()))
				}
				if id, ok := n.Fun.(*ast.Ident); ok && pauseWriters[id.Name] && !(inRalphloop && (base == "pauseticket.go" || base == "claim.go")) {
					t.Errorf("%s: calls %s outside the pause path", fset.Position(n.Pos()), id.Name)
				}
			case *ast.AssignStmt:
				for _, rhs := range n.Rhs {
					if isPauseStatus(rhs) && !(inRalphloop && base == "claim.go") {
						t.Errorf("%s: assigns a pause status outside claim.go's pause writers", fset.Position(n.Pos()))
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

func isPauseStatus(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && pauseStatuses[sel.Sel.Name]
}

// isSetStatusToPause catches the generic helper being used to pause, e.g.
// SetStatus(path, "paused (repair)").
func isSetStatusToPause(c *ast.CallExpr) bool {
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
	return ok && (lit.Value == `"paused"`)
}
