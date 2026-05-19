package worktrees

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
)

func TestSplitHeight_StackedLayout(t *testing.T) {
	m := Model{width: 80, height: 30} // width <= 100 → stacked
	m.worktrees = make([]git.Worktree, 5)
	tableH, sidebarH := m.splitHeight(30)
	if tableH+sidebarH != 30 {
		t.Errorf("tableH(%d) + sidebarH(%d) != total(30)", tableH, sidebarH)
	}
	if tableH < 4 {
		t.Errorf("tableH should be at least 4, got %d", tableH)
	}
	if sidebarH < 1 {
		t.Errorf("sidebarH should be at least 1, got %d", sidebarH)
	}
}

func TestSplitHeight_NonStackedLayout(t *testing.T) {
	m := Model{width: 200, height: 40} // width > 100 → side-by-side
	tableH, sidebarH := m.splitHeight(40)
	if tableH != 40 || sidebarH != 40 {
		t.Errorf("expected both to be 40, got tableH=%d sidebarH=%d", tableH, sidebarH)
	}
}

func TestContentHeight_MinFour(t *testing.T) {
	m := Model{height: 2}
	if h := m.contentHeight(); h != 4 {
		t.Errorf("expected min contentHeight=4, got %d", h)
	}
}

func TestContentHeight_Normal(t *testing.T) {
	m := Model{height: 30}
	if h := m.contentHeight(); h != 29 {
		t.Errorf("expected contentHeight=29 (30-1 helpLine), got %d", h)
	}
}

// Regression: table.SetRows with empty rows clamps cursor to -1 (len(rows)-1).
// If worktrees is non-empty when sidebarContent is subsequently called,
// m.worktrees[m.table.Cursor()] panics with index out of range [-1].
func TestWindowSizeMsgWithNegativeTableCursorDoesNotPanic(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, "")
	m.worktrees = []git.Worktree{
		{Name: "main", Path: filepath.Join(repoDir, "main"), Branch: repo.MainBranch},
		{Name: "feature-a", Path: filepath.Join(repoDir, "feature-a"), Branch: "feature-a"},
	}

	// Drive cursor to -1: SetRows with non-empty rows first (cursor=0 > -1),
	// then SetRows with empty rows clamps cursor to len([])-1 = -1.
	resizeTable(&m.table, 100, 10)
	m.table.SetRows(m.buildRows())
	m.table.SetRows([]table.Row{}) // cursor → -1

	if m.table.Cursor() != -1 {
		t.Fatalf("expected cursor=-1 after SetRows(empty), got %d", m.table.Cursor())
	}

	// Must not panic.
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
}
