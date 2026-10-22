package server_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func TestLandDecide_HeldTrunkLockBlocksNeitherClaimNorPollButDefersTheStamp(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	commitOnBranch(t, repo, "landed")
	writeCompleteEpic(t, store, "landed")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{"landed"}})
	servertest.WriteTicketWith(t, store, "proj", "other", "01", "o", servertest.TicketOpts{})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	launched := make(chan string, 4)
	h.RegisterLaunch(func(p servertest.Prompt) {
		epic, _, _ := strings.Cut(strings.TrimPrefix(p.Address, "proj:"), "/")
		commitWork(t, p, epic)
		launched <- p.Address
	})
	release, err := h.Server.HoldTrunkLock("proj", "main")
	if err != nil {
		t.Fatal(err)
	}
	runGitIn(t, repo, "merge", "-q", "--ff-only", "landed")
	if res, err := h.Client.QueueAdd(context.Background(), "proj:other/01", "claude"); err != nil || res.Refused {
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
	time.Sleep(200 * time.Millisecond)
	md := filepath.Join(store, "proj", "landed", "ticket.md")
	if strings.Contains(readFile(t, md), "merged_at:") {
		t.Error("merged_at stamped while the trunk lock was held")
	}
	release()
	waitUntil(t, "merged_at after release", func() bool { return strings.Contains(readFile(t, md), "merged_at:") })
}

func TestLandDecide_SecondCompleteSendsNoSecondNoticeOrRetro(t *testing.T) {
	chat, count := chatRecorder(t)
	f := retroFixture(t, withEvolve(config.EvolveContributor), chat)
	servertest.SetProjectRepoNoAutoMerge(t, f.store, "proj", f.repo)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")
	if got := waitForChat(count, epicNotMergedText); got != 1 {
		t.Fatalf("%q sent %d times, want once", epicNotMergedText, got)
	}

	if err := f.h.Server.CompleteRootIfDone("proj:" + schedEpic + "/01"); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")
	time.Sleep(10 * testFlushInterval)

	if got := count(epicNotMergedText); got != 1 {
		t.Errorf("%q sent %d times, want once", epicNotMergedText, got)
	}
	if got := f.retrosAfter(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}

func TestLandDecide_RepeatNeedsRebaseParkIsQuiet(t *testing.T) {
	chat, count := chatRecorder(t)
	f := newSchedFixture(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), func(f *schedFixture, p servertest.Prompt, id string) {
		if strings.Contains(p.Address, ":one-offs/") {
			return
		}
		commitWork(t, p, id)
		testutil.WriteFile(t, f.repo, "moved.txt", "moved")
		testutil.CommitAll(t, f.repo, "main moves on")
	}, withEvolve(config.EvolveContributor), chat)
	f.parkRoot(t)
	if got := waitForChat(count, epicNotMergedText); got != 1 {
		t.Fatalf("%q sent %d times, want once", epicNotMergedText, got)
	}
	parks := count("needs rebase")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.subscribe(ctx, t)
	if err := f.h.Server.CompleteRootIfDone("proj:" + schedEpic + "/01"); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventRootPaused, "")
	time.Sleep(10 * testFlushInterval)

	if parks != 1 {
		t.Errorf("first park notice sent %d times, want once", parks)
	}
	if got := count("needs rebase"); got != parks {
		t.Errorf("park notice sent %d times after the repeat, want %d", got, parks)
	}
	if got := count(epicNotMergedText); got != 1 {
		t.Errorf("%q sent %d times, want once", epicNotMergedText, got)
	}
	if got := f.retrosAfter(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}
