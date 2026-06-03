package git_test

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestSyncStatusPretty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		s    git.SyncStatus
		want string
	}{
		{git.SyncStatus{Name: git.StatusSame}, "synced"},
		{git.SyncStatus{Name: git.StatusAhead, Ahead: 3}, "3 ahead"},
		{git.SyncStatus{Name: git.StatusBehind, Behind: 2}, "2 behind"},
		{git.SyncStatus{Name: git.StatusDiverged, Ahead: 1, Behind: 4}, "1 ahead, 4 behind"},
		{git.SyncStatus{Name: git.StatusUnknown}, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.Pretty(); got != tt.want {
			t.Errorf("Pretty() = %q, want %q (status=%s)", got, tt.want, tt.s.Name)
		}
	}
}

func TestBranchSyncStatusAgainstRef_EmptyRefs(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	status, err := git.BranchSyncStatusAgainstRef(repoDir, "", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Name != git.StatusUnknown {
		t.Errorf("expected StatusUnknown for empty localRef, got %q", status.Name)
	}
}

func evalDir(t *testing.T, dir string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", dir, err)
	}
	return real
}

func tempBareRepoLight(t *testing.T) string {
	t.Helper()
	src := testutil.TempRepo(t)
	bare := filepath.Join(evalDir(t, t.TempDir()), "repo.git")
	testutil.MustGitExported(t, ".", "clone", "--bare", src, bare)
	return bare
}

func tempBareRepoWithWorktreesLight(t *testing.T, names ...string) string {
	t.Helper()
	repoDir := tempBareRepoLight(t)
	for _, name := range names {
		wtDir := filepath.Join(repoDir, name)
		testutil.MustGitExported(t, repoDir, "worktree", "add", "-b", name, wtDir)
		testutil.MustGitExported(t, wtDir, "config", "user.email", "test@test.com")
		testutil.MustGitExported(t, wtDir, "config", "user.name", "Test")
		testutil.WriteFile(t, wtDir, "file.txt", name)
		testutil.MustGitExported(t, wtDir, "add", ".")
		testutil.MustGitExported(t, wtDir, "commit", "-m", "add "+name)
	}
	return repoDir
}

func TestUncommittedChanges_clean(t *testing.T) {
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "feature")
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
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "feature")
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
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "feature")
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

func TestWorktreeStatusSummary_clean(t *testing.T) {
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")

	staged, unstaged, untracked, err := git.WorktreeStatusSummary(wtDir)
	if err != nil {
		t.Fatalf("WorktreeStatusSummary: %v", err)
	}
	if staged != 0 || unstaged != 0 || untracked != 0 {
		t.Errorf("expected all zeros for clean repo, got staged=%d unstaged=%d untracked=%d", staged, unstaged, untracked)
	}
}

func TestWorktreeStatusSummary_staged(t *testing.T) {
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")

	testutil.WriteFile(t, wtDir, "new.txt", "staged content")
	testutil.MustGitExported(t, wtDir, "add", "new.txt")

	staged, unstaged, untracked, err := git.WorktreeStatusSummary(wtDir)
	if err != nil {
		t.Fatalf("WorktreeStatusSummary: %v", err)
	}
	if staged != 1 {
		t.Errorf("expected staged=1, got %d", staged)
	}
	if unstaged != 0 || untracked != 0 {
		t.Errorf("expected unstaged=0 untracked=0, got unstaged=%d untracked=%d", unstaged, untracked)
	}
}

func TestWorktreeStatusSummary_unstaged(t *testing.T) {
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")

	testutil.WriteFile(t, wtDir, "file.txt", "modified")

	staged, unstaged, untracked, err := git.WorktreeStatusSummary(wtDir)
	if err != nil {
		t.Fatalf("WorktreeStatusSummary: %v", err)
	}
	if unstaged != 1 {
		t.Errorf("expected unstaged=1, got %d", unstaged)
	}
	if staged != 0 || untracked != 0 {
		t.Errorf("expected staged=0 untracked=0, got staged=%d untracked=%d", staged, untracked)
	}
}

func TestWorktreeStatusSummary_untracked(t *testing.T) {
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")

	testutil.WriteFile(t, wtDir, "newfile.txt", "untracked")

	staged, unstaged, untracked, err := git.WorktreeStatusSummary(wtDir)
	if err != nil {
		t.Fatalf("WorktreeStatusSummary: %v", err)
	}
	if untracked != 1 {
		t.Errorf("expected untracked=1, got %d", untracked)
	}
	if staged != 0 || unstaged != 0 {
		t.Errorf("expected staged=0 unstaged=0, got staged=%d unstaged=%d", staged, unstaged)
	}
}

func TestWorktreeStatusSummary_mixed(t *testing.T) {
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")

	// Staged: new file added to index
	testutil.WriteFile(t, wtDir, "staged.txt", "staged")
	testutil.MustGitExported(t, wtDir, "add", "staged.txt")
	// Unstaged: existing file modified without staging
	testutil.WriteFile(t, wtDir, "file.txt", "modified")
	// Untracked: new file not added
	testutil.WriteFile(t, wtDir, "untracked.txt", "untracked")

	staged, unstaged, untracked, err := git.WorktreeStatusSummary(wtDir)
	if err != nil {
		t.Fatalf("WorktreeStatusSummary: %v", err)
	}
	if staged != 1 {
		t.Errorf("expected staged=1, got %d", staged)
	}
	if unstaged != 1 {
		t.Errorf("expected unstaged=1, got %d", unstaged)
	}
	if untracked != 1 {
		t.Errorf("expected untracked=1, got %d", untracked)
	}
}

func TestWorktreeSyncStatus_aheadOfUpstream(t *testing.T) {
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "feature")
	// Set feature to track main so there is a configured upstream.
	testutil.SetBranchUpstream(t, repoDir, "feature", "main")
	repo, _ := git.FindRepo(repoDir)

	status, err := git.WorktreeSyncStatus(*repo, "feature")
	if err != nil {
		t.Fatalf("WorktreeSyncStatus: %v", err)
	}
	// feature has 1 commit ahead of main
	if status.Name != git.StatusAhead {
		t.Errorf("Status = %q, want %q", status.Name, git.StatusAhead)
	}
	if status.Ahead != 1 {
		t.Errorf("Ahead = %d, want 1", status.Ahead)
	}
}

func TestWorktreeSyncStatus_noUpstream(t *testing.T) {
	t.Parallel()
	repoDir := tempBareRepoWithWorktreesLight(t, "feature")
	repo, _ := git.FindRepo(repoDir)

	status, err := git.WorktreeSyncStatus(*repo, "feature")
	if err != nil {
		t.Fatalf("WorktreeSyncStatus: %v", err)
	}
	// No upstream configured -> unknown
	if status.Name != git.StatusUnknown {
		t.Errorf("Status = %q, want %q", status.Name, git.StatusUnknown)
	}
}

func TestWorktreeSyncStatus_main(t *testing.T) {
	t.Parallel()
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
