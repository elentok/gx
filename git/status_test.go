package git_test

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestUncommittedChanges_clean(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")

	changes, err := git.UncommittedChanges(wtDir)
	if err != nil {
		t.Fatalf("UncommittedChanges: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("got %d changes in clean worktree, want 0", len(changes))
	}
}

func TestUncommittedChanges_modified(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")

	testutil.WriteFile(t, wtDir, "file.txt", "modified")

	changes, err := git.UncommittedChanges(wtDir)
	if err != nil {
		t.Fatalf("UncommittedChanges: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	found := false
	for _, c := range changes {
		if c.Path == "file.txt" && c.Kind == git.ChangeModified {
			found = true
		}
	}
	if !found {
		t.Errorf("expected modified file.txt, got: %+v", changes)
	}
}

func TestUncommittedChanges_untracked(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")

	testutil.WriteFile(t, wtDir, "new.txt", "untracked")

	changes, err := git.UncommittedChanges(wtDir)
	if err != nil {
		t.Fatalf("UncommittedChanges: %v", err)
	}

	found := false
	for _, c := range changes {
		if c.Path == "new.txt" && c.Kind == git.ChangeUntracked {
			found = true
		}
	}
	if !found {
		t.Errorf("expected untracked new.txt, got: %+v", changes)
	}
}

func TestWorktreeSyncStatus_aheadOfUpstream(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	// Set feature to track origin/main so there is a configured upstream.
	testutil.SetBranchUpstream(t, repoDir, "feature", "origin/main")
	repo, _ := git.FindRepo(repoDir)

	status, err := git.WorktreeSyncStatus(*repo, "feature")
	if err != nil {
		t.Fatalf("WorktreeSyncStatus: %v", err)
	}
	// feature has 1 commit ahead of origin/main
	if status.Name != git.StatusAhead {
		t.Errorf("Status = %q, want %q", status.Name, git.StatusAhead)
	}
	if status.Ahead != 1 {
		t.Errorf("Ahead = %d, want 1", status.Ahead)
	}
}

func TestWorktreeSyncStatus_noUpstream(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	repo, _ := git.FindRepo(repoDir)

	status, err := git.WorktreeSyncStatus(*repo, "feature")
	if err != nil {
		t.Fatalf("WorktreeSyncStatus: %v", err)
	}
	// No upstream configured → unknown
	if status.Name != git.StatusUnknown {
		t.Errorf("Status = %q, want %q", status.Name, git.StatusUnknown)
	}
}

func TestWorktreeSyncStatus_main(t *testing.T) {
	repoDir := testutil.TempBareRepo(t)
	repo, _ := git.FindRepo(repoDir)

	status, err := git.WorktreeSyncStatus(*repo, "main")
	if err != nil {
		t.Fatalf("WorktreeSyncStatus: %v", err)
	}
	if status.Name != git.StatusSame {
		t.Errorf("Status = %q, want %q", status.Name, git.StatusSame)
	}
}
