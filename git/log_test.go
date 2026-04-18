package git_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestCommitsSinceMain_hasCommits(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	repo, _ := git.FindRepo(repoDir)

	commits, err := git.CommitsSinceMain(*repo, "feature")
	if err != nil {
		t.Fatalf("CommitsSinceMain: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1", len(commits))
	}
	if commits[0].Subject != "add feature" {
		t.Errorf("Subject = %q, want %q", commits[0].Subject, "add feature")
	}
	if commits[0].Hash == "" {
		t.Error("Hash is empty")
	}
}

func TestCommitsSinceMain_noCommits(t *testing.T) {
	repoDir := testutil.TempBareRepo(t)
	repo, _ := git.FindRepo(repoDir)

	commits, err := git.CommitsSinceMain(*repo, "main")
	if err != nil {
		t.Fatalf("CommitsSinceMain: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("got %d commits for main vs main, want 0", len(commits))
	}
}

func TestCommitsSinceMain_multipleCommits(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")
	repo, _ := git.FindRepo(repoDir)

	// Add a second commit to the feature branch
	testutil.WriteFile(t, wtDir, "extra.txt", "extra")
	testutil.CommitAll(t, wtDir, "second commit")

	commits, err := git.CommitsSinceMain(*repo, "feature")
	if err != nil {
		t.Fatalf("CommitsSinceMain: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2", len(commits))
	}
}

func TestCommitsBehindMain(t *testing.T) {
	dir := testutil.TempRepo(t)
	repo, _ := git.FindRepo(dir)

	mustGit(t, dir, "checkout", "-b", "feature")
	testutil.WriteFile(t, dir, "feature.txt", "feature")
	testutil.CommitAll(t, dir, "feature commit")

	mustGit(t, dir, "checkout", "main")
	testutil.WriteFile(t, dir, "main.txt", "main")
	testutil.CommitAll(t, dir, "main commit")

	behind, err := git.CommitsBehindMain(*repo, "feature")
	if err != nil {
		t.Fatalf("CommitsBehindMain: %v", err)
	}
	if len(behind) != 1 {
		t.Fatalf("got %d commits behind main, want 1", len(behind))
	}
	if behind[0].Subject != "main commit" {
		t.Fatalf("got behind subject %q, want %q", behind[0].Subject, "main commit")
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
