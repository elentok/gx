package worktrees

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

// Regression: a reload used to unconditionally snap the cursor back to the
// worktree the user launched from (activeWorktreePath) on every
// worktreesLoadedMsg, discarding whatever the user had selected.
func TestWorktreesLoadedMsg_PreservesSelectionAcrossReload(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	wts := []git.Worktree{
		{Name: "main", Path: filepath.Join(repoDir, "main"), Branch: repo.MainBranch},
		{Name: "feature-a", Path: filepath.Join(repoDir, "feature-a"), Branch: "feature-a"},
		{Name: "feature-b", Path: filepath.Join(repoDir, "feature-b"), Branch: "feature-b"},
	}

	// activeWorktreePath is "main" (where the user launched from), but the
	// user has since selected "feature-b".
	m := New(*repo, wts[0].Path)
	next, _ := m.Update(worktreesLoadedMsg{worktrees: wts})
	m = next.(Model)

	m.table.SetCursor(2) // feature-b

	next, _ = m.Update(worktreesLoadedMsg{worktrees: wts})
	m = next.(Model)

	if got := m.worktrees[m.table.Cursor()].Name; got != "feature-b" {
		t.Fatalf("expected selection to stay on feature-b after reload, got %q", got)
	}
}

// First load has no prior selection, so it should fall back to the worktree
// the user launched from.
func TestWorktreesLoadedMsg_FirstLoadSelectsActiveWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	wts := []git.Worktree{
		{Name: "main", Path: filepath.Join(repoDir, "main"), Branch: repo.MainBranch},
		{Name: "feature-a", Path: filepath.Join(repoDir, "feature-a"), Branch: "feature-a"},
	}

	m := New(*repo, wts[1].Path)
	next, _ := m.Update(worktreesLoadedMsg{worktrees: wts})
	m = next.(Model)

	if got := m.worktrees[m.table.Cursor()].Name; got != "feature-a" {
		t.Fatalf("expected first load to select active worktree feature-a, got %q", got)
	}
}
