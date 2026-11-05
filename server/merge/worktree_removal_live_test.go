package merge_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// newLiveWorkFixture is newFFFixture with session holders enabled.
func newLiveWorkFixture(t *testing.T) *schedFixture {
	t.Helper()
	return newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
	}, servertest.WithTestHolder())
}

func (f *schedFixture) sessionIn(ctx context.Context, t *testing.T, dir string) string {
	t.Helper()
	res, err := f.H.Client.CreateSession(ctx, wire.CreateSessionRequest{Project: "proj", Dir: dir, Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	return res.Session.ID
}

func TestWorktreeRemoval_LiveSessionKeepsTheWorktreeUntilItStops(t *testing.T) {
	t.Parallel()
	f := newLiveWorkFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs, err := f.H.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	completedUnmerged(ctx, t, f)
	id := f.sessionIn(ctx, t, f.featureWorktree(t))

	if res, err := f.H.Client.EpicMerge(ctx, "proj:"+schedEpic); err != nil || !res.Merged {
		t.Fatalf("merge = %+v, %v, want a direct merge", res, err)
	}
	f.settle(t)
	f.assertFeatureKept(t, "the merge, with a live session")
	f.assertKeptLogged(t, id)

	if _, err := f.H.Client.StopSession(ctx, id); err != nil {
		t.Fatal(err)
	}
	waitForSession(ctx, t, f.H, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	f.H.Server.Reconciler().RemoveMergedWorktreeSettled("proj", schedEpic)
	f.settle(t)
	f.assertWorktreeRemoved(t)
}

func TestWorktreeRemoval_ExitedSessionDoesNotKeepTheWorktree(t *testing.T) {
	t.Parallel()
	f := newLiveWorkFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs, err := f.H.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	completedUnmerged(ctx, t, f)
	id := f.sessionIn(ctx, t, f.featureWorktree(t))
	if _, err := f.H.Client.StopSession(ctx, id); err != nil {
		t.Fatal(err)
	}
	waitForSession(ctx, t, f.H, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })

	if res, err := f.H.Client.EpicMerge(ctx, "proj:"+schedEpic); err != nil || !res.Merged {
		t.Fatalf("merge = %+v, %v, want a direct merge", res, err)
	}
	f.settle(t)
	f.assertWorktreeRemoved(t)
}

func TestWorktreeRemoval_LiveRunInTheWorktreeKeepsItAndNamesTheRun(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	f, _ := newMergeFixtureWith(t, func(_ *schedFixture, _ string, _ int) string {
		started <- struct{}{}
		<-release
		return ""
	})
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f.QueueEpic(ctx, t, "01")
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("the merge ticket's agent never started")
	}
	f.stampMergedByHand(t)

	f.H.Server.Reconciler().RemoveMergedWorktreeSettled("proj", schedEpic)

	waitUntil(t, "the kept line", func() bool {
		return strings.Contains(f.serverLog(t), "kept merged epic's feature worktree")
	})
	f.assertFeatureKept(t, "the removal")
	f.assertKeptLogged(t, "run proj:"+schedEpic+"/01")
}

// A merge ticket's finish triggers the merge before settleRun drops its entry,
// so the entry alone must not count.
func TestWorktreeRemoval_StoppedRunStillRegisteredDoesNotKeepTheWorktree(t *testing.T) {
	t.Parallel()
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)
	f.H.PutRunIn("proj:"+schedEpic+"/99", "proj:"+schedEpic, f.featureWorktree(t), false)

	if res, err := f.H.Client.EpicMerge(ctx, "proj:"+schedEpic); err != nil || !res.Merged {
		t.Fatalf("merge = %+v, %v, want a direct merge", res, err)
	}
	f.settle(t)
	f.assertWorktreeRemoved(t)
}

func TestWorktreeRemoval_RunWithoutCwdKeepsTheWorktreeOfItsEpic(t *testing.T) {
	t.Parallel()
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)
	f.H.PutRunIn("proj:"+schedEpic+"/99", "proj:"+schedEpic, "", true)

	if res, err := f.H.Client.EpicMerge(ctx, "proj:"+schedEpic); err != nil || !res.Merged {
		t.Fatalf("merge = %+v, %v, want a direct merge", res, err)
	}
	f.settle(t)
	f.assertFeatureKept(t, "the merge, with an older run of the epic")
	f.assertKeptLogged(t, "run proj:"+schedEpic+"/99")
}

func TestWorktreeRemoval_RunInASiblingWorktreeDoesNotKeepTheWorktree(t *testing.T) {
	t.Parallel()
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)
	// Named to share the feature worktree's prefix, so only whole path elements tell them apart.
	sibling := f.featureWorktree(t) + "-iter"
	f.Git(t, "worktree", "add", "-q", "--detach", sibling)
	f.H.PutRunIn("proj:"+schedEpic+"/99", "proj:"+schedEpic, sibling, true)

	if res, err := f.H.Client.EpicMerge(ctx, "proj:"+schedEpic); err != nil || !res.Merged {
		t.Fatalf("merge = %+v, %v, want a direct merge", res, err)
	}
	f.settle(t)
	if _, err := os.Stat(f.featureWorktree(t)); !os.IsNotExist(err) {
		t.Errorf("feature worktree still on disk (err %v)", err)
	}
}

func TestWorktreeRemoval_FinishedMergeTicketDoesNotKeepTheWorktree(t *testing.T) {
	t.Parallel()
	f, _ := newMergeFixture(t, func(int) string { return "finished" })
	servertest.SetProjectRepo(t, f.Store, "proj", f.Repo)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	f.settle(t)
	f.assertWorktreeRemoved(t)
}
