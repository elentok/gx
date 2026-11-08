package cleanup_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func branches(t *testing.T, repo string) string {
	return gitIn(t, repo, "branch", "--format=%(refname:short)")
}

func startCleanup(t *testing.T, repo string) *servertest.Harness {
	t.Helper()
	store := t.TempDir()
	if err := servertest.WriteEpic(store, "proj", ""); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	return servertest.StartWithStore(t, store)
}

func commitFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.CommitAll(t, dir, "add "+name)
}

func TestCleanupWrite_RemoveWorktreeLeavesNoFolderAndNoBranch(t *testing.T) {
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "feature-x")
	h := startCleanup(t, repo)

	res, err := h.Client.Reads.RemoveCleanupWorktree(context.Background(), wire.CleanupWriteRequest{Project: "proj", Branch: "feature-x"})
	if err != nil || res.Refused {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("worktree folder still there: %v", err)
	}
	if strings.Contains(gitIn(t, repo, "worktree", "list"), "feature-x") || strings.Contains(branches(t, repo), "feature-x") {
		t.Errorf("worktree or branch left behind")
	}
}

func TestCleanupWrite_SquashMergedBranchIsDeletedWithForce(t *testing.T) {
	repo := testutil.TempRepo(t)
	trunk := strings.TrimSpace(gitIn(t, repo, "branch", "--show-current"))
	gitIn(t, repo, "switch", "-c", "squashed")
	commitFile(t, repo, "s.txt")
	gitIn(t, repo, "switch", trunk)
	gitIn(t, repo, "merge", "--squash", "squashed")
	testutil.CommitAll(t, repo, "squash")
	h := startCleanup(t, repo)

	res, err := h.Client.Reads.DeleteCleanupBranch(context.Background(), wire.CleanupWriteRequest{Project: "proj", Branch: "squashed"})
	if err != nil || res.Refused {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if strings.Contains(branches(t, repo), "squashed") {
		t.Errorf("branch left behind")
	}
}

func TestCleanupWrite_CherryPickedBranchIsDeletedWithForce(t *testing.T) {
	repo := testutil.TempRepo(t)
	trunk := strings.TrimSpace(gitIn(t, repo, "branch", "--show-current"))
	gitIn(t, repo, "switch", "-c", "picked")
	commitFile(t, repo, "p.txt")
	sha := strings.TrimSpace(gitIn(t, repo, "rev-parse", "HEAD"))
	gitIn(t, repo, "switch", trunk)
	gitIn(t, repo, "cherry-pick", sha)
	h := startCleanup(t, repo)

	res, err := h.Client.Reads.DeleteCleanupBranch(context.Background(), wire.CleanupWriteRequest{Project: "proj", Branch: "picked"})
	if err != nil || res.Refused {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if strings.Contains(branches(t, repo), "picked") {
		t.Errorf("branch left behind")
	}
}

func TestCleanupWrite_IterationBranchMergedIntoEpicBranchIsDeletedWithForce(t *testing.T) {
	repo := testutil.TempRepo(t)
	trunk := strings.TrimSpace(gitIn(t, repo, "branch", "--show-current"))
	gitIn(t, repo, "switch", "-c", "myepic")
	gitIn(t, repo, "switch", "-c", "ralph-loop/myepic-item-01")
	commitFile(t, repo, "i.txt")
	sha := strings.TrimSpace(gitIn(t, repo, "rev-parse", "HEAD"))
	gitIn(t, repo, "switch", "myepic")
	gitIn(t, repo, "cherry-pick", sha)
	gitIn(t, repo, "switch", trunk)
	store := t.TempDir()
	if err := servertest.WriteEpic(store, "proj", "myepic"); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.Reads.DeleteCleanupBranch(context.Background(), wire.CleanupWriteRequest{Project: "proj", Branch: "ralph-loop/myepic-item-01"})
	if err != nil || res.Refused {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if strings.Contains(branches(t, repo), "myepic-item-01") {
		t.Errorf("branch left behind")
	}
}

func TestCleanupWrite_RefusesWhatIsNotProvenMerged(t *testing.T) {
	repo := testutil.TempRepo(t)
	trunk := strings.TrimSpace(gitIn(t, repo, "branch", "--show-current"))
	gitIn(t, repo, "switch", "-c", "extra")
	commitFile(t, repo, "x.txt")
	gitIn(t, repo, "switch", trunk)
	h := startCleanup(t, repo)

	res, err := h.Client.Reads.DeleteCleanupBranch(context.Background(), wire.CleanupWriteRequest{Project: "proj", Branch: "extra"})
	if err != nil || !res.Refused || res.Reason != wire.ReasonCleanupNotMerged {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if !strings.Contains(branches(t, repo), "extra") {
		t.Errorf("unmerged branch deleted")
	}
}

func TestCleanupWrite_RefusesWhenTheStateChangedSinceTheScan(t *testing.T) {
	repo := testutil.TempRepo(t)
	gitIn(t, repo, "branch", "gone")
	h := startCleanup(t, repo)
	ctx := context.Background()
	if _, err := h.Client.Reads.Cleanup(ctx, "proj"); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "branch", "-d", "gone")

	res, err := h.Client.Reads.DeleteCleanupBranch(ctx, wire.CleanupWriteRequest{Project: "proj", Branch: "gone"})
	if err != nil || !res.Refused || res.Reason != wire.ReasonCleanupUnknown {
		t.Errorf("res = %+v, err = %v", res, err)
	}
}

func TestCleanupWrite_RouteMustMatchWhetherTheBranchHasAWorktree(t *testing.T) {
	repo, _ := testutil.TempRepoWithLinkedWorktree(t, "feature-x")
	gitIn(t, repo, "branch", "bare-branch")
	h := startCleanup(t, repo)
	ctx := context.Background()

	res, _ := h.Client.Reads.DeleteCleanupBranch(ctx, wire.CleanupWriteRequest{Project: "proj", Branch: "feature-x"})
	if res.Reason != wire.ReasonCleanupHasWorktree {
		t.Errorf("delete on a worktree branch = %+v", res)
	}
	res, _ = h.Client.Reads.RemoveCleanupWorktree(ctx, wire.CleanupWriteRequest{Project: "proj", Branch: "bare-branch"})
	if res.Reason != wire.ReasonCleanupNoWorktree {
		t.Errorf("remove on a branch without worktree = %+v", res)
	}
}

func TestCleanupWrite_GitsOwnReasonComesBackForARefusedRow(t *testing.T) {
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "feature-x")
	if err := os.WriteFile(filepath.Join(wt, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := startCleanup(t, repo)

	res, err := h.Client.Reads.RemoveCleanupWorktree(context.Background(), wire.CleanupWriteRequest{Project: "proj", Branch: "feature-x"})
	if err != nil || !res.Refused || res.Reason != wire.ReasonCleanupGitRefused || !strings.Contains(res.Message, "untracked") {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("worktree gone: %v", err)
	}
	if !strings.Contains(branches(t, repo), "feature-x") {
		t.Errorf("branch deleted before its worktree went")
	}
}

func TestCleanupWrite_WaitsForTheWorktreeLock(t *testing.T) {
	repo := testutil.TempRepo(t)
	gitIn(t, repo, "branch", "waiting")
	h := startCleanup(t, repo)

	worktree.Lock.Lock()
	done := make(chan wire.CleanupWriteResult, 1)
	go func() {
		res, _ := h.Client.Reads.DeleteCleanupBranch(context.Background(), wire.CleanupWriteRequest{Project: "proj", Branch: "waiting"})
		done <- res
	}()
	select {
	case <-done:
		worktree.Lock.Unlock()
		t.Fatal("write ran while the lock was held")
	case <-time.After(300 * time.Millisecond):
	}
	if !strings.Contains(branches(t, repo), "waiting") {
		t.Error("branch deleted under the held lock")
	}
	worktree.Lock.Unlock()
	select {
	case res := <-done:
		if res.Refused {
			t.Errorf("res = %+v", res)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("write never ran after the lock was released")
	}
}

func removeUnmerged(t *testing.T, h *servertest.Harness) wire.CleanupWriteResult {
	t.Helper()
	res, err := h.Client.Reads.RemoveUnmergedCleanupWorktree(context.Background(), wire.CleanupWriteRequest{Project: "proj", Branch: "feature-x"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestCleanupWrite_UnmergedCleanIdleWorktreeIsRemovedAndTheBranchKept(t *testing.T) {
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "feature-x")
	commitFile(t, wt, "recent.txt")
	h := startCleanup(t, repo)

	if res := removeUnmerged(t, h); res.Refused || !res.WorktreeRemoved {
		t.Fatalf("res = %+v", res)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("worktree folder still there: %v", err)
	}
	if !strings.Contains(branches(t, repo), "feature-x") {
		t.Errorf("branch was deleted")
	}
}

func TestCleanupWrite_UnmergedDirtyWorktreeIsRefused(t *testing.T) {
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "feature-x")
	commitFile(t, wt, "recent.txt")
	if err := os.WriteFile(filepath.Join(wt, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := startCleanup(t, repo)

	if res := removeUnmerged(t, h); !res.Refused || res.Reason != wire.ReasonCleanupDirty {
		t.Fatalf("res = %+v", res)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("worktree gone: %v", err)
	}
}

func TestCleanupWrite_UnmergedWorktreeWithALiveSessionIsRefused(t *testing.T) {
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "feature-x")
	commitFile(t, wt, "recent.txt")
	store := t.TempDir()
	if err := servertest.WriteEpic(store, "proj", ""); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder())
	res, err := h.Client.Sessions.Create(context.Background(), wire.CreateSessionRequest{Project: "proj", Dir: wt, Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}

	if got := removeUnmerged(t, h); !got.Refused || got.Reason != wire.ReasonCleanupLive {
		t.Fatalf("res = %+v", got)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("worktree gone: %v", err)
	}
}

func TestCleanupRead_ChangesNothing(t *testing.T) {
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "feature-x")
	h := startCleanup(t, repo)
	before := gitIn(t, repo, "worktree", "list") + branches(t, repo)

	if _, err := h.Client.Reads.Cleanup(context.Background(), "proj"); err != nil {
		t.Fatal(err)
	}
	if after := gitIn(t, repo, "worktree", "list") + branches(t, repo); after != before {
		t.Errorf("read changed the repo:\n%s\nvs\n%s", before, after)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("worktree gone: %v", err)
	}
}
