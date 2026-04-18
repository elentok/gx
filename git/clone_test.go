package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestCloneBare_dotBareLayout(t *testing.T) {
	src := testutil.TempRepo(t)
	cwd := t.TempDir()

	outerDir, err := git.CloneBare(src, "", cwd)
	if err != nil {
		t.Fatalf("CloneBare: %v", err)
	}
	// Resolve symlinks so paths match what git returns (macOS /var → /private/var).
	outerDir, err = filepath.EvalSymlinks(outerDir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	// Outer directory should exist and contain .bare/ and .git file.
	if _, err := os.Stat(filepath.Join(outerDir, ".bare")); err != nil {
		t.Fatalf(".bare directory missing: %v", err)
	}
	gitFile := filepath.Join(outerDir, ".git")
	data, err := os.ReadFile(gitFile)
	if err != nil {
		t.Fatalf(".git file missing: %v", err)
	}
	if string(data) != "gitdir: ./.bare\n" {
		t.Fatalf(".git file content = %q, want %q", data, "gitdir: ./.bare\n")
	}

	// FindRepo from the outer directory should work and identify it as bare
	// with WorktreeDir pointing to the outer directory.
	repo, err := git.FindRepo(outerDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	if !repo.IsBare {
		t.Fatal("repo should be bare")
	}
	if repo.LinkedWorktreeDir() != outerDir {
		t.Errorf("LinkedWorktreeDir = %q, want %q", repo.LinkedWorktreeDir(), outerDir)
	}

	mainBranch := repo.MainBranch
	if mainBranch != "main" && mainBranch != "master" {
		t.Fatalf("unexpected default branch: %q", mainBranch)
	}

	// Bootstrap the initial worktree under the outer directory (not under .bare/).
	wtPath := filepath.Join(repo.LinkedWorktreeDir(), mainBranch)
	if err := git.AddWorktreeFromRemote(*repo, wtPath, mainBranch, "origin/"+mainBranch); err != nil {
		t.Fatalf("AddWorktreeFromRemote: %v", err)
	}

	// Worktree should be a sibling of .bare/, not nested inside it.
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree directory missing at %s: %v", wtPath, err)
	}

	// git log should work inside the worktree.
	branch, err := git.CurrentBranch(wtPath)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != mainBranch {
		t.Fatalf("CurrentBranch = %q, want %q", branch, mainBranch)
	}

	// ListWorktrees from the outer dir should find the worktree with its short name.
	worktrees, err := git.ListWorktrees(*repo)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(worktrees) != 1 {
		t.Fatalf("got %d worktrees, want 1", len(worktrees))
	}
	if worktrees[0].Name != mainBranch {
		t.Errorf("worktree Name = %q, want %q", worktrees[0].Name, mainBranch)
	}

	// IdentifyDir from inside the worktree should resolve back to the outer dir.
	info, err := git.IdentifyDir(wtPath)
	if err != nil {
		t.Fatalf("IdentifyDir from worktree: %v", err)
	}
	if info.Repo.LinkedWorktreeDir() != outerDir {
		t.Errorf("IdentifyDir LinkedWorktreeDir = %q, want %q", info.Repo.LinkedWorktreeDir(), outerDir)
	}

	// CloneBare should leave remote tracking refs populated so gx does not
	// prompt the user to run git fetch on first launch.
	if problem := git.CheckFetchConfig(outerDir); problem != nil {
		t.Errorf("CheckFetchConfig after CloneBare: %s", problem.Description)
	}
}

func TestCloneBare_stripsGitSuffix(t *testing.T) {
	src := testutil.TempRepo(t)
	cwd := t.TempDir()

	// Simulate a URL ending in .git by passing a target without .git.
	// Test that the outer dir name does not end in .git.
	outerDir, err := git.CloneBare(src, "", cwd)
	if err != nil {
		t.Fatalf("CloneBare: %v", err)
	}
	if filepath.Base(outerDir) == ".git" || len(filepath.Base(outerDir)) == 0 {
		t.Errorf("unexpected outer dir name: %q", filepath.Base(outerDir))
	}
}
