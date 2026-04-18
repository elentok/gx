package git_test

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestListWorktrees_empty(t *testing.T) {
	repoDir := testutil.TempBareRepo(t)
	repo, _ := git.FindRepo(repoDir)

	wts, err := git.ListWorktrees(*repo)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	// Bare repo with no linked worktrees should return empty
	if len(wts) != 0 {
		t.Errorf("got %d worktrees, want 0", len(wts))
	}
}

func TestListWorktrees_withWorktrees(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	repo, _ := git.FindRepo(repoDir)

	wts, err := git.ListWorktrees(*repo)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 2 {
		t.Fatalf("got %d worktrees, want 2", len(wts))
	}

	names := map[string]bool{}
	for _, wt := range wts {
		names[wt.Name] = true
		if wt.Branch == "" {
			t.Errorf("worktree %q has empty Branch", wt.Name)
		}
		if wt.Head == "" {
			t.Errorf("worktree %q has empty Head", wt.Name)
		}
	}
	if !names["feature-a"] {
		t.Error("missing worktree feature-a")
	}
	if !names["feature-b"] {
		t.Error("missing worktree feature-b")
	}
}

func TestListWorktrees_paths(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "my-feature")
	repo, _ := git.FindRepo(repoDir)

	wts, err := git.ListWorktrees(*repo)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("got %d worktrees, want 1", len(wts))
	}

	wt := wts[0]
	if wt.Name != "my-feature" {
		t.Errorf("Name = %q, want %q", wt.Name, "my-feature")
	}
	if wt.Path != filepath.Join(repoDir, "my-feature") {
		t.Errorf("Path = %q, want %q", wt.Path, filepath.Join(repoDir, "my-feature"))
	}
	if wt.Branch != "my-feature" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "my-feature")
	}
}

func TestAddWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepo(t)
	repo, _ := git.FindRepo(repoDir)

	newPath := filepath.Join(repoDir, "feature")
	if err := git.AddWorktree(*repo, "feature", newPath, "main"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	wts, err := git.ListWorktrees(*repo)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("got %d worktrees after add, want 1", len(wts))
	}
	if wts[0].Name != "feature" {
		t.Errorf("Name = %q, want %q", wts[0].Name, "feature")
	}
	if wts[0].Branch != "feature" {
		t.Errorf("Branch = %q, want %q", wts[0].Branch, "feature")
	}
}

func TestRemoveWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "to-delete", "keep")
	repo, _ := git.FindRepo(repoDir)

	if err := git.RemoveWorktree(*repo, "to-delete", false); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}

	wts, _ := git.ListWorktrees(*repo)
	for _, wt := range wts {
		if wt.Name == "to-delete" {
			t.Error("worktree to-delete still exists after removal")
		}
	}
}
