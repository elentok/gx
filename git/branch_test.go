package git_test

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestListBranches(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestIsRebasedOnMain_NoMainBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	repo := git.Repo{Root: dir}
	ok, err := git.IsRebasedOnMain(repo, "main")
	if err != nil {
		t.Fatalf("IsRebasedOnMain: %v", err)
	}
	if !ok {
		t.Error("expected true when MainBranch is empty")
	}
}

func TestIsRebasedOnMain_SameBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	repo := git.Repo{Root: dir, MainBranch: "main"}
	ok, err := git.IsRebasedOnMain(repo, "main")
	if err != nil {
		t.Fatalf("IsRebasedOnMain: %v", err)
	}
	if !ok {
		t.Error("expected true when branch == MainBranch")
	}
}

func TestIsRebasedOnMain_True(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.MustGitExported(t, dir, "checkout", "-b", "feature")
	testutil.WriteFile(t, dir, "feature.txt", "feature\n")
	testutil.CommitAll(t, dir, "feature commit")
	testutil.MustGitExported(t, dir, "checkout", "main")

	repo := git.Repo{Root: dir, MainBranch: "main"}
	ok, err := git.IsRebasedOnMain(repo, "feature")
	if err != nil {
		t.Fatalf("IsRebasedOnMain: %v", err)
	}
	if !ok {
		t.Error("expected true: feature was branched directly from main")
	}
}

func TestIsRebasedOnMain_False(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	testutil.MustGitExported(t, dir, "checkout", "-b", "feature")
	testutil.WriteFile(t, dir, "feature.txt", "feature\n")
	testutil.CommitAll(t, dir, "feature commit")
	testutil.MustGitExported(t, dir, "checkout", "main")
	testutil.WriteFile(t, dir, "extra.txt", "extra\n")
	testutil.CommitAll(t, dir, "main extra commit")

	repo := git.Repo{Root: dir, MainBranch: "main"}
	ok, err := git.IsRebasedOnMain(repo, "feature")
	if err != nil {
		t.Fatalf("IsRebasedOnMain: %v", err)
	}
	if ok {
		t.Error("expected false: feature is not rebased on new main commit")
	}
}

func TestTrackRemote(t *testing.T) {
	t.Parallel()
	dir := testutil.TempBareRepo(t)
	// Create a new branch with no upstream set, then track remote
	testutil.MustGitExported(t, dir, "branch", "test-track", "main")
	if err := git.TrackRemote(dir, "origin", "main"); err != nil {
		t.Fatalf("TrackRemote: %v", err)
	}
	// Verify the explicit upstream is set (for-each-ref, not the fallback)
	if up := git.UpstreamBranch(dir, "main"); up != "origin/main" {
		t.Errorf("expected origin/main upstream, got %q", up)
	}
}

func TestDeleteRemoteBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.TempBareRepo(t)
	repo, err := git.FindRepo(dir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	// Push a non-checked-out feature branch to origin (a regular TempRepo)
	testutil.MustGitExported(t, dir, "branch", "feature", "main")
	testutil.MustGitExported(t, dir, "push", "origin", "feature")

	if err := git.DeleteRemoteBranch(*repo, "origin", "feature"); err != nil {
		t.Fatalf("DeleteRemoteBranch: %v", err)
	}

	testutil.MustGitExported(t, dir, "fetch", "--prune", "origin")
	branches, _ := git.ListBranches(*repo)
	for _, b := range branches {
		if b.IsRemote && b.RemoteName == "origin" && b.Name == "feature" {
			t.Error("feature branch still exists on remote after deletion")
		}
	}
}
