package server_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestTrunkLock_TwoEpicsFinishingTogetherMergeOneAndPauseTheOtherAsNonFF(t *testing.T) {
	const other = "other"
	f := newSchedFixture(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), func(_ *schedFixture, p servertest.Prompt, id string) {
		epic, _, _ := strings.Cut(strings.TrimPrefix(p.Address, "proj:"), "/")
		commitWork(t, p, epic+"-"+id)
	})
	for id, opts := range atMergeCap(map[string]servertest.TicketOpts{"01": {}}) {
		servertest.WriteTicketWith(t, f.store, "proj", other, id, "t", opts)
	}
	f.h.Server.Rescan()
	var inFlight, overlapped atomic.Int32
	f.h.Server.SetMergeInto(func(repo git.Repo, wts []git.Worktree, branch, target string) (git.MergeOutcome, error) {
		if inFlight.Add(1) > 1 {
			overlapped.Add(1)
		}
		defer inFlight.Add(-1)
		// Long enough for the other epic to finish and try its merge meanwhile.
		time.Sleep(time.Second)
		return git.MergeBranchInto(repo, wts, branch, target)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	if res, err := f.h.Client.QueueAdd(ctx, "proj:"+other+"/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	var completed, paused []server.Event
	for len(completed)+len(paused) < 2 {
		select {
		case ev := <-evs:
			switch ev.Type {
			case server.EventIterationFailed:
				t.Fatalf("iteration-failed: %+v", ev)
			case server.EventRootCompleted:
				completed = append(completed, ev)
			case server.EventRootPaused:
				paused = append(paused, ev)
			}
		case <-ctx.Done():
			t.Fatalf("roots did not settle: completed %+v, paused %+v", completed, paused)
		}
	}

	if overlapped.Load() > 0 {
		t.Error("two merges into main ran at once")
	}
	if len(completed) != 1 || len(paused) != 1 {
		t.Fatalf("completed %+v, paused %+v, want one each", completed, paused)
	}
	if p := paused[0]; p.Kind != "" || !strings.HasPrefix(p.Message, "needs rebase: ") {
		t.Errorf("pause = kind %q, reason %q, want a needs-rebase pause", p.Kind, p.Message)
	}
	merged := strings.TrimPrefix(completed[0].Address, "proj:")
	if got := f.git(t, "rev-parse", "main"); got != f.git(t, "rev-parse", merged) {
		t.Errorf("main at %s, want %s's tip", got, merged)
	}
}

func TestTrunkLock_AlreadyMergedEpicDoesNoGitWorkAndNotifiesOnce(t *testing.T) {
	chat, count := chatRecorder(t)
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		// A person merged the epic while its last ticket ran.
		if err := tickets.StampEpicFields(filepath.Join(f.store, "proj", schedEpic), map[string]any{"merged_at": time.Now().UTC()}); err != nil {
			t.Error(err)
		}
	}, chat)
	f.h.Server.SetMergeInto(func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error) {
		t.Error("merge core called for an already merged epic")
		return git.MergeOutcome{}, errors.New("unexpected")
	})
	main := f.git(t, "rev-parse", "main")

	f.runEpic(t, server.EventRootCompleted)

	if got := f.git(t, "rev-parse", "main"); got != main {
		t.Errorf("main moved to %s", got)
	}
	q, err := f.h.Client.QueueItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 0 {
		t.Errorf("queue = %+v, want the root dequeued", q)
	}
	if got := waitForChat(count, epicMergedText); got != 1 {
		t.Errorf("%q sent %d times, want once", epicMergedText, got)
	}
	f.assertStamps(t, true)
}

func TestTrunkLock_TrunkMovedBeforeTheFFPausesNeedsRebaseNotMergeFailed(t *testing.T) {
	f := newSchedFixture(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), func(_ *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
	})
	f.h.Server.SetMergeInto(func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error) {
		// Git's ff check passed, then trunk moved and the ff itself failed.
		testutil.WriteFile(t, f.repo, "moved.txt", "moved")
		testutil.CommitAll(t, f.repo, "main moves on")
		return git.MergeOutcome{}, errors.New("cannot lock ref 'refs/heads/main'")
	})

	ev := f.pauseRoot(t)

	if ev.Kind != "" || ev.Message != "needs rebase: "+schedEpic+" onto main; rebase it, then merge again" {
		t.Errorf("pause = kind %q, reason %q, want a needs-rebase pause", ev.Kind, ev.Message)
	}
	f.assertRootQueued(t)
	f.assertStamps(t, false)
}
