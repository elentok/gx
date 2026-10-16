package log

import (
	"testing"

	"github.com/elentok/gx/ui"
)

func TestDetailImageDiffOriginIncludesChromeRows(t *testing.T) {
	t.Parallel()
	m := setupSplitWithDetail(t)
	m, _ = m.withSyncedDetailOrigin()

	col, row, visible := m.split.DetailOrigin()
	if !visible {
		t.Fatal("expected detail pane visible in split mode")
	}
	atZero, _ := m.commitDetail.WithScreenOrigin(0, 0, true)
	relCol, relRow, _, _, ok := atZero.PanelGeometry()
	if !ok {
		t.Fatal("expected commit detail to have a diff pane geometry")
	}

	gotCol, gotRow, _, _, ok := m.commitDetail.PanelGeometry()
	if !ok {
		t.Fatal("expected synced commit detail to have a diff pane geometry")
	}
	if gotCol != col+relCol || gotRow != ui.ChromeRows+row+relRow {
		t.Fatalf("image origin = (%d,%d), want (%d,%d)", gotCol, gotRow, col+relCol, ui.ChromeRows+row+relRow)
	}
}
