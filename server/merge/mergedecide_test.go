package merge_test

import (
	"context"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func TestMergeDecide_HeldTrunkLockBlocksNeitherClaimNorPollButDefersTheStamp(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	commitOnBranch(t, repo, "landed")
	writeCompleteEpic(t, store, "landed")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"landed"}})
	servertest.WriteTicketWith(t, store, "proj", "other", "01", "o", servertest.TicketOpts{})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	launched := make(chan string, 4)
	h.OnTurn(func(p *servertest.Turn) {
		epic, _, _ := strings.Cut(strings.TrimPrefix(p.Address, "proj:"), "/")
		commitWork(t, p, epic)
		launched <- p.Address
	})
	release, err := h.Server.Merger().HoldTrunkLock("proj", "main")
	if err != nil {
		t.Fatal(err)
	}
	runGitIn(t, repo, "merge", "-q", "--ff-only", "landed")
	if res, err := h.Client.Queue.Add(context.Background(), "proj:other/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	select {
	case got := <-launched:
		if got != "proj:other/01" {
			t.Fatalf("launched %s, want proj:other/01", got)
		}
	case <-time.After(10 * time.Second):
		release()
		t.Fatal("no claim while the trunk lock was held")
	}
	time.Sleep(200 * time.Millisecond) // negative wait: a stamp would land within a few polls
	md := filepath.Join(store, "proj", "landed", "ticket.md")
	if strings.Contains(servertest.ReadFile(t, md), "merged_at:") {
		t.Error("merged_at stamped while the trunk lock was held")
	}
	release()
	waitUntil(t, "merged_at after release", func() bool { return strings.Contains(servertest.ReadFile(t, md), "merged_at:") })
}

func TestMergeDecide_SecondCompleteSendsNoSecondNoticeOrRetro(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	f := retroFixture(t, rigs.WithEvolve(config.EvolveContributor), chat)
	servertest.SetProjectRepoNoAutoMerge(t, f.Store, "proj", f.Repo)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")
	if got := waitForChat(count, epicNotMergedText); got != 1 {
		t.Fatalf("%q sent %d times, want once", epicNotMergedText, got)
	}

	if err := f.H.Server.Merger().CompleteRootOf("proj:" + schedEpic + "/01"); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")
	time.Sleep(3 * testFlushInterval) // negative wait: a repeat notice would flush within a few intervals

	if got := count(epicNotMergedText); got != 1 {
		t.Errorf("%q sent %d times, want once", epicNotMergedText, got)
	}
	if got := f.RetrosAfter(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}

func TestMergeDecide_RepeatNeedsRebasePauseIsQuiet(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	f := newSchedFixture(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), func(f *schedFixture, p *servertest.Turn, id string) {
		if strings.Contains(p.Address, ":one-offs/") {
			return
		}
		commitWork(t, p, id)
		testutil.WriteFile(t, f.Repo, "moved.txt", "moved")
		testutil.CommitAll(t, f.Repo, "main moves on")
	}, rigs.WithEvolve(config.EvolveContributor), chat)
	f.pauseRoot(t)
	if got := waitForChat(count, epicNotMergedText); got != 1 {
		t.Fatalf("%q sent %d times, want once", epicNotMergedText, got)
	}
	pauses := count("needs rebase")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.Subscribe(ctx, t)
	if err := f.H.Server.Merger().CompleteRootOf("proj:" + schedEpic + "/01"); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootPaused, "")
	time.Sleep(3 * testFlushInterval) // negative wait: a repeat notice would flush within a few intervals

	if pauses != 1 {
		t.Errorf("first pause notice sent %d times, want once", pauses)
	}
	if got := count("needs rebase"); got != pauses {
		t.Errorf("pause notice sent %d times after the repeat, want %d", got, pauses)
	}
	if got := count(epicNotMergedText); got != 1 {
		t.Errorf("%q sent %d times, want once", epicNotMergedText, got)
	}
	if got := f.RetrosAfter(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}
