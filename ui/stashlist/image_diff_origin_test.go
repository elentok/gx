package stashlist

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func TestDetailImageDiffOriginIncludesChromeRows(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	mustStashBigChange(t, repo)

	m := runModelInit(NewModel(repo, ui.Settings{}, keys.Manager{}))
	m = sendModel(m, tea.WindowSizeMsg{Width: 200, Height: 40})
	m, _ = m.withSyncedDetailOrigin()

	col, row, visible := m.split.DetailOrigin()
	if !visible {
		t.Fatal("expected detail pane visible")
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
