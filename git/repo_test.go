package git_test

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestFindRepo_standardRepo(t *testing.T) {
	dir := testutil.TempRepo(t)

	repo, err := git.FindRepo(dir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	if repo.Root != dir {
		t.Errorf("Root = %q, want %q", repo.Root, dir)
	}
	if repo.IsBare {
		t.Error("IsBare = true, want false")
	}
}

func TestFindRepo_standardRepo_subdir(t *testing.T) {
	dir := testutil.TempRepo(t)
	sub := filepath.Join(dir, "sub")
	testutil.Mkdir(t, sub)

	repo, err := git.FindRepo(sub)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	if repo.Root != dir {
		t.Errorf("Root = %q, want %q", repo.Root, dir)
	}
}

func TestFindRepo_bareRepo(t *testing.T) {
	dir := testutil.TempBareRepo(t)

	repo, err := git.FindRepo(dir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	if repo.Root != dir {
		t.Errorf("Root = %q, want %q", repo.Root, dir)
	}
	if !repo.IsBare {
		t.Error("IsBare = false, want true")
	}
}

func TestIdentifyDir_worktreeRoot(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")

	info, err := git.IdentifyDir(wtDir)
	if err != nil {
		t.Fatalf("IdentifyDir: %v", err)
	}
	if !info.IsWorktreeRoot {
		t.Error("IsWorktreeRoot = false, want true")
	}
	if info.IsRepoRoot {
		t.Error("IsRepoRoot = true, want false")
	}
	if info.Repo.Root != repoDir {
		t.Errorf("Repo.Root = %q, want %q", info.Repo.Root, repoDir)
	}
	if !info.Repo.IsBare {
		t.Error("Repo.IsBare = false, want true")
	}
	if info.WorktreeRoot != wtDir {
		t.Errorf("WorktreeRoot = %q, want %q", info.WorktreeRoot, wtDir)
	}
}

func TestIdentifyDir_bareRepoRoot(t *testing.T) {
	dir := testutil.TempBareRepo(t)

	info, err := git.IdentifyDir(dir)
	if err != nil {
		t.Fatalf("IdentifyDir: %v", err)
	}
	if !info.IsRepoRoot {
		t.Error("IsRepoRoot = false, want true")
	}
	if info.IsWorktreeRoot {
		t.Error("IsWorktreeRoot = true, want false")
	}
}

func TestFindRepo_notARepo(t *testing.T) {
	dir := t.TempDir()
	_, err := git.FindRepo(dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDetectMainBranch(t *testing.T) {
	dir := testutil.TempBareRepo(t)
	repo, err := git.FindRepo(dir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	if repo.MainBranch != "main" {
		t.Errorf("MainBranch = %q, want %q", repo.MainBranch, "main")
	}
}
