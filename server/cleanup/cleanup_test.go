package cleanup_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

func TestCleanupRead_SizesLandInTheBackgroundAndPublishAnEvent(t *testing.T) {
	store := t.TempDir()
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "feature-x")
	if err := servertest.WriteEpic(store, "proj", ""); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	first, err := h.Client.Reads.Cleanup(ctx, "proj")
	if err != nil {
		t.Fatal(err)
	}
	got := findWorktree(first, wt)
	if got == nil {
		t.Fatalf("worktree %s missing from %+v", wt, first.Worktrees)
	}
	if got.SizeBytes != nil {
		t.Errorf("first read already has a size: %d", *got.SizeBytes)
	}

	for ev := range evs {
		if ev.Type == wire.EventCleanupSizeChanged && ev.Address == "proj" {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("no cleanup-size-changed event")
		}
	}
	second, err := h.Client.Reads.Cleanup(ctx, "proj")
	if err != nil {
		t.Fatal(err)
	}
	got = findWorktree(second, wt)
	if got == nil || got.SizeBytes == nil || *got.SizeBytes == 0 {
		t.Errorf("size after the event = %+v", got)
	}
}

func TestCleanupRead_UnknownProjectIs404(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	_, err := h.Client.Reads.Cleanup(context.Background(), "nope")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want a 404", err)
	}
}

func actionsByBranch(t *testing.T, h *servertest.Harness) map[string]string {
	t.Helper()
	rep, err := h.Client.Reads.Cleanup(context.Background(), "proj")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, wt := range rep.Worktrees {
		got[wt.Branch] = wt.Action
	}
	return got
}

func TestCleanupRead_FreshSquashMergedBranchGetsTheMergedDelete(t *testing.T) {
	repo := testutil.TempRepo(t)
	trunk := strings.TrimSpace(gitIn(t, repo, "branch", "--show-current"))
	gitIn(t, repo, "switch", "-c", "squashed")
	commitFile(t, repo, "s.txt")
	gitIn(t, repo, "switch", trunk)
	gitIn(t, repo, "merge", "--squash", "squashed")
	testutil.CommitAll(t, repo, "squash")
	h := startCleanup(t, repo)

	if got := actionsByBranch(t, h)["squashed"]; got != wire.CleanupActionMergedDelete {
		t.Errorf("action = %q, want %q", got, wire.CleanupActionMergedDelete)
	}
}

func TestCleanupRead_RecentCommitUnmergedCleanWorktreeGetsTheUnmergedRemove(t *testing.T) {
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "feature-x")
	commitFile(t, wt, "x.txt")
	h := startCleanup(t, repo)

	if got := actionsByBranch(t, h)["feature-x"]; got != wire.CleanupActionUnmergedRemove {
		t.Errorf("action = %q, want %q", got, wire.CleanupActionUnmergedRemove)
	}
}

func TestCleanupRead_DetachedWorktreeGetsNoAction(t *testing.T) {
	repo := testutil.TempRepo(t)
	detached := filepath.Join(repo, ".worktrees", "detached")
	gitIn(t, repo, "worktree", "add", "--detach", detached)
	h := startCleanup(t, repo)

	rep, err := h.Client.Reads.Cleanup(context.Background(), "proj")
	if err != nil {
		t.Fatal(err)
	}
	wt := findWorktree(rep, detached)
	if wt == nil || wt.Action != "" {
		t.Errorf("detached row = %+v, want no action", wt)
	}
}

func findWorktree(rep wire.CleanupReport, path string) *wire.CleanupWorktree {
	for i := range rep.Worktrees {
		if rep.Worktrees[i].Path == path {
			return &rep.Worktrees[i]
		}
	}
	return nil
}
