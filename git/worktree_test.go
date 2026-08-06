package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestListWorktrees_empty(t *testing.T) {
	t.Parallel()
	repoDir := tempBareRepoLight(t)
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
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "feature-a", "feature-b")
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
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "my-feature")
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
	t.Parallel()
	repoDir := tempBareRepoLight(t)
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

func TestAddWorktree_standardRepo_underDotWorktrees(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	newPath := filepath.Join(repo.LinkedWorktreeDir(), "feature")
	if err := git.AddWorktree(*repo, "feature", newPath, "main"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	wantPath := filepath.Join(repoDir, ".worktrees", "feature")
	if newPath != wantPath {
		t.Fatalf("newPath = %q, want %q", newPath, wantPath)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("worktree not created at %q: %v", newPath, err)
	}

	excludePath := filepath.Join(repoDir, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("reading %s: %v", excludePath, err)
	}
	found := false
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == ".worktrees/" {
			found = true
		}
	}
	if !found {
		t.Errorf("%s does not contain %q, got: %q", excludePath, ".worktrees/", string(data))
	}

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if strings.Contains(string(out), ".worktrees") {
		t.Errorf("git status shows .worktrees as untracked: %q", string(out))
	}
}

func TestRemoveWorktree(t *testing.T) {
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "to-delete", "keep")
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

func TestMoveWorktree(t *testing.T) {
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "my-branch")
	repo, _ := git.FindRepo(repoDir)

	oldPath := filepath.Join(repoDir, "my-branch")
	newPath := filepath.Join(repoDir, "my-branch-moved")

	if err := git.MoveWorktree(*repo, oldPath, newPath); err != nil {
		t.Fatalf("MoveWorktree: %v", err)
	}

	wts, err := git.ListWorktrees(*repo)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	found := false
	for _, wt := range wts {
		if wt.Path == oldPath {
			t.Error("old path still exists in worktrees after move")
		}
		if wt.Path == newPath {
			found = true
		}
	}
	if !found {
		t.Error("new path not found in worktrees after move")
	}
}
