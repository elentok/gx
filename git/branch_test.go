package git_test

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestListBranches(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	repo, _ := git.FindRepo(repoDir)

	branches, err := git.ListBranches(*repo)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}

	local := map[string]bool{}
	for _, b := range branches {
		if !b.IsRemote {
			local[b.Name] = true
		}
	}

	for _, want := range []string{"main", "feature-a", "feature-b"} {
		if !local[want] {
			t.Errorf("missing local branch %q", want)
		}
	}
}

func TestDeleteLocalBranch(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "to-delete")
	repo, _ := git.FindRepo(repoDir)

	// Remove the worktree first so we can delete the branch
	if err := git.RemoveWorktree(*repo, "to-delete", false); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	// force=true because the branch has commits not in main
	if err := git.DeleteLocalBranch(*repo, "to-delete", true); err != nil {
		t.Fatalf("DeleteLocalBranch: %v", err)
	}

	branches, _ := git.ListBranches(*repo)
	for _, b := range branches {
		if b.Name == "to-delete" && !b.IsRemote {
			t.Error("branch to-delete still exists after deletion")
		}
	}
}

func TestRenameBranch(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "old-name")
	repo, _ := git.FindRepo(repoDir)

	if err := git.RenameBranch(*repo, "old-name", "new-name"); err != nil {
		t.Fatalf("RenameBranch: %v", err)
	}

	branches, _ := git.ListBranches(*repo)
	found := false
	for _, b := range branches {
		if b.Name == "old-name" && !b.IsRemote {
			t.Error("branch old-name still exists after rename")
		}
		if b.Name == "new-name" && !b.IsRemote {
			found = true
		}
	}
	if !found {
		t.Error("branch new-name not found after rename")
	}
}

func TestParseBranchLine_local(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, _ := git.FindRepo(repoDir)

	// Create a second branch so we have something to list
	if err := git.CreateBranch(*repo, "other"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	branches, err := git.ListBranches(*repo)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}

	found := false
	for _, b := range branches {
		if b.Name == "other" && !b.IsRemote {
			found = true
			if b.GitName != "other" {
				t.Errorf("GitName = %q, want %q", b.GitName, "other")
			}
		}
	}
	if !found {
		t.Error("branch 'other' not found")
	}
}
