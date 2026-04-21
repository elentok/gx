package git_test

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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
	if commits[0].Date.IsZero() {
		t.Error("Date is zero")
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

func TestBranchHistorySinceMain_NoUpstreamMarksLocalCommits(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	repo, _ := git.FindRepo(repoDir)

	history, err := git.BranchHistorySinceMain(*repo, "feature", "")
	if err != nil {
		t.Fatalf("BranchHistorySinceMain: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("got %d commits, want 1", len(history))
	}
	if history[0].Class != git.BranchHistoryLocalOnly {
		t.Fatalf("got class %q, want %q", history[0].Class, git.BranchHistoryLocalOnly)
	}
	if history[0].Date.IsZero() {
		t.Fatal("expected populated date")
	}
}

func TestBranchHistorySinceMain_UsesRemoteMainNotLocalMain(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	repo, _ := git.FindRepo(repoDir)
	mainDir := filepath.Join(repoDir, "main")
	mustGit(t, repoDir, "worktree", "add", mainDir, "main")
	mustGit(t, mainDir, "config", "user.email", "test@test.com")
	mustGit(t, mainDir, "config", "user.name", "Test")

	testutil.WriteFile(t, mainDir, "local-main-only.txt", "not pushed\n")
	testutil.CommitAll(t, mainDir, "local main only")

	history, err := git.BranchHistorySinceMain(*repo, "feature", "")
	if err != nil {
		t.Fatalf("BranchHistorySinceMain: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("got %d commits, want 1", len(history))
	}
	if history[0].Subject != "add feature" {
		t.Fatalf("got subject %q, want %q", history[0].Subject, "add feature")
	}
}

func TestBranchHistorySinceMain_DivergedIncludesRemoteOnly(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	repo, _ := git.FindRepo(repoDir)
	featureDir := filepath.Join(repoDir, "feature")

	testutil.PushBranchWithUpstream(t, featureDir, "origin", "feature")
	mustGit(t, featureDir, "reset", "--hard", "HEAD~1")
	testutil.WriteFile(t, featureDir, "shared.txt", "shared\n")
	testutil.CommitAll(t, featureDir, "shared commit")
	testutil.WriteFile(t, featureDir, "local.txt", "local\n")
	testutil.CommitAll(t, featureDir, "local only")
	testutil.MustGitExported(t, featureDir, "push", "--force-with-lease", "-u", "origin", "feature")

	testutil.WriteFile(t, featureDir, "remote.txt", "remote\n")
	testutil.CommitAll(t, featureDir, "remote only")
	testutil.MustGitExported(t, featureDir, "push")

	mustGit(t, featureDir, "reset", "--hard", "HEAD~1")
	testutil.WriteFile(t, featureDir, "local2.txt", "local again\n")
	testutil.CommitAll(t, featureDir, "local again")

	history, err := git.BranchHistorySinceMain(*repo, "feature", "origin/feature")
	if err != nil {
		t.Fatalf("BranchHistorySinceMain: %v", err)
	}
	if len(history) < 3 {
		t.Fatalf("got %d commits, want at least 3", len(history))
	}

	got := map[string]git.BranchHistoryClass{}
	for _, commit := range history {
		got[commit.Subject] = commit.Class
	}
	if got["shared commit"] != git.BranchHistoryShared {
		t.Fatalf("shared commit class = %q, want %q", got["shared commit"], git.BranchHistoryShared)
	}
	if got["remote only"] != git.BranchHistoryRemoteOnly {
		t.Fatalf("remote only class = %q, want %q", got["remote only"], git.BranchHistoryRemoteOnly)
	}
	if got["local again"] != git.BranchHistoryLocalOnly {
		t.Fatalf("local again class = %q, want %q", got["local again"], git.BranchHistoryLocalOnly)
	}
	for i := 1; i < len(history); i++ {
		if history[i-1].Date.Before(history[i].Date) && !history[i-1].Date.Equal(history[i].Date) {
			t.Fatalf("history not sorted newest-first: %v before %v", history[i-1].Date, history[i].Date)
		}
	}
}

func TestHeadCommit_PopulatesFullHashAndDate(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	head, err := git.HeadCommit(repoDir, "main")
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	if head.FullHash == "" || len(head.FullHash) < len(head.Hash) {
		t.Fatalf("unexpected hashes: full=%q short=%q", head.FullHash, head.Hash)
	}
	if head.Date.IsZero() || time.Since(head.Date) < 0 {
		t.Fatalf("unexpected date: %v", head.Date)
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
