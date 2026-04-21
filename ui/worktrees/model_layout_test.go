package worktrees

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
)

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
