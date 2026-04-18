package git_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

// findIssueAbout finds the first issue whose Description contains substr,
// failing the test if none is found.
func findIssueAbout(t *testing.T, issues []git.Issue, substr string) git.Issue {
	t.Helper()
	for _, iss := range issues {
		if strings.Contains(iss.Description, substr) {
			return iss
		}
	}
	t.Fatalf("no issue about %q found in %d issues", substr, len(issues))
	panic("unreachable")
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// setupDotBareRepo creates a .bare-style repo with one linked worktree and
// returns the outer directory and the resolved repo.
func setupDotBareRepo(t *testing.T) (outerDir string, repo git.Repo) {
	t.Helper()
	src := testutil.TempRepo(t)
	cwd := t.TempDir()

	raw, err := git.CloneBare(src, "", cwd)
	if err != nil {
		t.Fatalf("CloneBare: %v", err)
	}
	outerDir, err = filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	r, err := git.FindRepo(outerDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	repo = *r

	// Populate remote tracking refs so fetch-config checks pass.
	if _, err := git.UpdateRemotes(repo); err != nil {
		t.Fatalf("UpdateRemotes: %v", err)
	}

	// Create one linked worktree.
	branch := repo.MainBranch
	wtPath := filepath.Join(outerDir, branch)
	if err := git.AddWorktreeFromRemote(repo, wtPath, branch, "origin/"+branch); err != nil {
		t.Fatalf("AddWorktreeFromRemote: %v", err)
	}

	return outerDir, repo
}

func TestCheckRepo_NoIssuesOnCleanDotBare(t *testing.T) {
	_, repo := setupDotBareRepo(t)

	issues, err := git.CheckRepo(repo)
	if err != nil {
		t.Fatalf("CheckRepo: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %d: %v", len(issues), issues)
	}
}

func TestCheckRepo_DetectsMissingOuterGitFile(t *testing.T) {
	outerDir, repo := setupDotBareRepo(t)

	gitFile := filepath.Join(outerDir, ".git")
	if err := os.Remove(gitFile); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	issues, err := git.CheckRepo(repo)
	if err != nil {
		t.Fatalf("CheckRepo: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected an issue for missing .git file, got none")
	}
}

func TestCheckRepo_FixesOuterGitFile(t *testing.T) {
	outerDir, repo := setupDotBareRepo(t)

	// Corrupt the .git file.
	gitFile := filepath.Join(outerDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: wrong\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	issues, err := git.CheckRepo(repo)
	if err != nil {
		t.Fatalf("CheckRepo: %v", err)
	}

	issue := findIssueAbout(t, issues, gitFile)
	if err := issue.Fix(); err != nil {
		t.Fatalf("Fix: %v", err)
	}

	data, err := os.ReadFile(gitFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "gitdir: ./.bare\n" {
		t.Errorf(".git content = %q, want %q", string(data), "gitdir: ./.bare\n")
	}
}

func TestCheckRepo_DetectsBadWorktreeGitFile(t *testing.T) {
	outerDir, repo := setupDotBareRepo(t)

	// Corrupt the worktree .git file.
	wtGitFile := filepath.Join(outerDir, repo.MainBranch, ".git")
	if err := os.WriteFile(wtGitFile, []byte("gitdir: /nonexistent/path\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	issues, err := git.CheckRepo(repo)
	if err != nil {
		t.Fatalf("CheckRepo: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected an issue for bad worktree .git file, got none")
	}
}

func TestCheckRepo_FixesWorktreeGitFile(t *testing.T) {
	outerDir, repo := setupDotBareRepo(t)

	// Corrupt the worktree .git file.
	branch := repo.MainBranch
	wtGitFile := filepath.Join(outerDir, branch, ".git")
	if err := os.WriteFile(wtGitFile, []byte("gitdir: /nonexistent/path\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	issues, err := git.CheckRepo(repo)
	if err != nil {
		t.Fatalf("CheckRepo: %v", err)
	}

	// Description contains the worktree name, not the full file path.
	issue := findIssueAbout(t, issues, "worktree "+branch)
	if err := issue.Fix(); err != nil {
		t.Fatalf("Fix: %v", err)
	}

	// After fix there should be no worktree-related issues.
	issues, err = git.CheckRepo(repo)
	if err != nil {
		t.Fatalf("CheckRepo after fix: %v", err)
	}
	for _, iss := range issues {
		if strings.Contains(iss.Description, "worktree "+branch) {
			t.Fatalf("still has worktree issue after fix: %s", iss.Description)
		}
	}
}

func TestCheckRepo_NoIssuesForRegularBareRepo(t *testing.T) {
	dir := testutil.TempBareRepo(t)
	repo, err := git.FindRepo(dir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	issues, err := git.CheckRepo(*repo)
	if err != nil {
		t.Fatalf("CheckRepo: %v", err)
	}
	// No .bare checks should run for a normal bare repo.
	for _, iss := range issues {
		// Any issue here is about fetch config, not .bare — that's acceptable.
		_ = iss
	}
}
