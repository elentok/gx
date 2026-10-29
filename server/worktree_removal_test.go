package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

const settleTimeout = 10 * time.Second

// holdWorktreeLock takes ralph-loop's worktree lock until the returned func runs.
func holdWorktreeLock(t *testing.T) (release func()) {
	t.Helper()
	acquired, done, stop := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_ = ralphloop.WithWorktreeLock(func() error {
			close(acquired)
			<-stop
			return nil
		})
	}()
	<-acquired
	var once bool
	release = func() {
		if !once {
			once = true
			close(stop)
			<-done
		}
	}
	t.Cleanup(release)
	return release
}

func (f *schedFixture) settle(t *testing.T) {
	t.Helper()
	if !f.h.Server.WaitSettled(settleTimeout) {
		t.Fatal("background work never settled")
	}
}

// assertWorktreeRemoved checks the feature worktree is gone from disk and from
// git, and the epic branch survived.
func (f *schedFixture) assertWorktreeRemoved(t *testing.T) {
	t.Helper()
	wt := f.featureWorktree(t)
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("feature worktree still on disk (err %v)", err)
	}
	if list := f.git(t, "worktree", "list"); strings.Contains(list, filepath.Base(wt)) {
		t.Errorf("git still lists the worktree:\n%s", list)
	}
	if strings.TrimSpace(f.git(t, "branch", "--list", schedEpic)) == "" {
		t.Errorf("branch %s was deleted", schedEpic)
	}
}

func (f *schedFixture) serverLog(t *testing.T) string {
	t.Helper()
	log, err := os.ReadFile(server.LogPath(f.h.StateDir))
	if err != nil {
		t.Fatal(err)
	}
	return string(log)
}

// assertKeptLogged checks one "kept" log line names the epic, the path and a reason containing want.
func (f *schedFixture) assertKeptLogged(t *testing.T, want string) {
	t.Helper()
	for _, line := range strings.Split(f.serverLog(t), "\n") {
		if !strings.Contains(line, "kept merged epic's feature worktree") {
			continue
		}
		for _, part := range []string{schedEpic, f.featureWorktree(t), want} {
			if !strings.Contains(line, part) {
				t.Errorf("kept line lacks %q:\n%s", part, line)
			}
		}
		return
	}
	t.Errorf("no kept line in the log:\n%s", f.serverLog(t))
}

// stampMergedByHand stamps merged_at on the epic as a merge would, without
// starting any removal.
func (f *schedFixture) stampMergedByHand(t *testing.T) {
	t.Helper()
	fields := map[string]any{"merged_at": time.Now().UTC()}
	if err := tickets.StampEpicFields(filepath.Join(f.store, "proj", schedEpic), fields); err != nil {
		t.Fatal(err)
	}
}

func TestWorktreeRemoval_FastForwardMergeRemovesTheWorktreeAndKeepsTheBranch(t *testing.T) {
	t.Parallel()
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")
	f.assertStamps(t, true)

	f.settle(t)
	f.assertWorktreeRemoved(t)
}

func TestWorktreeRemoval_NothingToMergeRemovesTheWorktree(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, _ *servertest.Turn, id string) {
		f.finishCommitless(t, id)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")
	f.assertStamps(t, true)

	f.settle(t)
	f.assertWorktreeRemoved(t)
}

func TestWorktreeRemoval_HandMergeRemovesTheWorktree(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	commitOnBranch(t, repo, "blocker")
	writeCompleteEpic(t, store, "blocker")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"blocker"}})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	wt := filepath.Join(h.Server.WorktreeDir("proj"), "blocker")
	runGitIn(t, repo, "worktree", "add", "-q", wt, "blocker")

	runGitIn(t, repo, "merge", "-q", "--ff-only", "blocker")

	waitUntil(t, "the worktree to be removed", func() bool {
		_, err := os.Stat(wt)
		return os.IsNotExist(err)
	})
	f := schedFixture{repo: repo}
	if strings.TrimSpace(f.git(t, "branch", "--list", "blocker")) == "" {
		t.Error("branch blocker was deleted")
	}
}

func TestWorktreeRemoval_ManualMergeRouteRemovesTheWorktree(t *testing.T) {
	t.Parallel()
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)
	f.assertFeatureKept(t, "completion without a merge")

	if res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic); err != nil || !res.Merged {
		t.Fatalf("merge = %+v, %v, want a direct merge", res, err)
	}

	f.settle(t)
	f.assertWorktreeRemoved(t)
}

func TestWorktreeRemoval_UntrackedFileKeepsTheWorktreeAndLogsWhy(t *testing.T) {
	t.Parallel()
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)
	testutil.WriteFile(t, f.featureWorktree(t), "scratch.txt", "keep me")

	if res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic); err != nil || !res.Merged {
		t.Fatalf("merge = %+v, %v, want a direct merge", res, err)
	}

	f.settle(t)
	f.assertFeatureKept(t, "the merge")
	f.assertKeptLogged(t, "untracked")
}

func TestWorktreeRemoval_QueuedRootKeepsTheWorktree(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		testutil.WriteFile(t, f.repo, id+".txt", "local")
	})
	f.pauseRoot(t)
	f.assertRootQueued(t)
	f.stampMergedByHand(t)

	f.h.Server.RemoveMergedWorktree("proj", schedEpic)

	f.settle(t)
	f.assertFeatureKept(t, "the removal")
	f.assertKeptLogged(t, "queued")
}

func TestWorktreeRemoval_OpenTicketKeepsTheWorktree(t *testing.T) {
	t.Parallel()
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)
	servertest.WriteTicketWith(t, f.store, "proj", schedEpic, "02", "late", servertest.TicketOpts{})
	f.stampMergedByHand(t)

	f.h.Server.RemoveMergedWorktree("proj", schedEpic)

	f.settle(t)
	f.assertFeatureKept(t, "the removal")
	f.assertKeptLogged(t, "02")
}

func TestWorktreeRemoval_OneOffsRootKeepsItsWorktree(t *testing.T) {
	t.Parallel()
	f := newFFFixture(t)
	wt := filepath.Join(f.h.Server.WorktreeDir("proj"), server.OneOffsEpic)
	f.git(t, "branch", "oneoffs-branch")
	f.git(t, "worktree", "add", "-q", wt, "oneoffs-branch")

	f.h.Server.RemoveMergedWorktree("proj", server.OneOffsEpic)

	f.settle(t)
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("one-offs worktree gone: %v", err)
	}
}

func TestWorktreeRemoval_CompletedUnmergedEpicKeepsItsWorktree(t *testing.T) {
	t.Parallel()
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)

	f.settle(t)
	f.assertFeatureKept(t, "completion without a merge")
}

// Serial: holds ralphloop's process-wide worktree lock, which would stall
// every parallel test that merges or prepares an iteration.
func TestWorktreeRemoval_WaitsForTheWorktreeLockAfterTheMergeDequeuedTheRoot(t *testing.T) {
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)
	release := holdWorktreeLock(t)

	if res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic); err != nil || !res.Merged {
		t.Fatalf("merge = %+v, %v, want a direct merge", res, err)
	}
	if f.rootQueued(t) {
		t.Error("root still queued after the merge returned")
	}
	f.assertFeatureKept(t, "the merge, while the worktree lock was held")

	release()
	f.settle(t)
	f.assertWorktreeRemoved(t)
}
