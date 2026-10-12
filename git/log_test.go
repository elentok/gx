package git_test

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestBranchHistorySinceTrunk_NoUpstreamMarksLocalCommits(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	repo, _ := git.FindRepo(repoDir)
	repo.Trunk = "main"

	history, err := git.BranchHistorySinceTrunk(*repo, "feature", "")
	if err != nil {
		t.Fatalf("BranchHistorySinceTrunk: %v", err)
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

func TestBranchHistorySinceTrunk_UsesRemoteMainNotLocalMain(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	repo, _ := git.FindRepo(repoDir)
	repo.Trunk = "main"
	mainDir := filepath.Join(repoDir, "main")
	mustGit(t, repoDir, "worktree", "add", mainDir, "main")
	mustGit(t, mainDir, "config", "user.email", "test@test.com")
	mustGit(t, mainDir, "config", "user.name", "Test")

	testutil.WriteFile(t, mainDir, "local-main-only.txt", "not pushed\n")
	testutil.CommitAll(t, mainDir, "local main only")

	history, err := git.BranchHistorySinceTrunk(*repo, "feature", "")
	if err != nil {
		t.Fatalf("BranchHistorySinceTrunk: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("got %d commits, want 1", len(history))
	}
	if history[0].Subject != "add feature" {
		t.Fatalf("got subject %q, want %q", history[0].Subject, "add feature")
	}
}

func TestTrunkBaseRef(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	mustGit(t, repoDir, "branch", "develop", "main")

	cases := map[string]string{"main": "origin/main", "develop": "develop", "nope": "", "": ""}
	for trunk, want := range cases {
		if got := git.TrunkBaseRef(repoDir, trunk); got != want {
			t.Errorf("TrunkBaseRef(%q) = %q, want %q", trunk, got, want)
		}
	}
}

func TestBranchHistorySinceTrunk_UsesConfiguredTrunk(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	mustGit(t, repoDir, "branch", "develop", "feature")
	repo, _ := git.FindRepo(repoDir)
	repo.Trunk = "develop"

	history, err := git.BranchHistorySinceTrunk(*repo, "feature", "")
	if err != nil {
		t.Fatalf("BranchHistorySinceTrunk: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("got %d commits since develop, want 0", len(history))
	}
}

func TestBranchHistorySinceTrunk_DivergedIncludesRemoteOnly(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	repo, _ := git.FindRepo(repoDir)
	repo.Trunk = "main"
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

	history, err := git.BranchHistorySinceTrunk(*repo, "feature", "origin/feature")
	if err != nil {
		t.Fatalf("BranchHistorySinceTrunk: %v", err)
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
	t.Parallel()
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

func TestIsLocalBranch_Exists(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	if !git.IsLocalBranch(repoDir, "main") {
		t.Error("expected IsLocalBranch=true for 'main'")
	}
}

func TestIsLocalBranch_Missing(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	if git.IsLocalBranch(repoDir, "nonexistent-branch-xyz") {
		t.Error("expected IsLocalBranch=false for nonexistent branch")
	}
}

func TestCommitsBetween_ReturnsNewCommits(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	repo, _ := git.FindRepo(repoDir)
	repo.Trunk = "main"

	commits, err := git.CommitsBetween(*repo, "main", "feature")
	if err != nil {
		t.Fatalf("CommitsBetween: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if commits[0].Subject != "add feature" {
		t.Errorf("Subject = %q, want 'add feature'", commits[0].Subject)
	}
}

func TestLogFilterIsActive(t *testing.T) {
	t.Parallel()
	empty := git.LogFilter{}
	if empty.IsActive() {
		t.Error("empty LogFilter should not be active")
	}
	withPath := git.LogFilter{Path: "main.go"}
	if !withPath.IsActive() {
		t.Error("LogFilter with path should be active")
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
