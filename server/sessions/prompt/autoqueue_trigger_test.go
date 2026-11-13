package prompt_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

// moveMapToBranch rewrites the map's step through the store, then rescans. A
// ping first makes the server see the old step, so the move is a transition and
// not a first sight.
func moveMapToBranch(t *testing.T, f *rigs.GrillingFixture, store string) {
	t.Helper()
	if err := f.H.Client.Tickets.Changed(f.Ctx, "proj:map"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store, "proj", "map", "ticket.md")
	if err := os.WriteFile(path, []byte("---\nkind: map\ndesign_step: branch\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.H.Client.Tickets.Changed(f.Ctx, "proj:map"); err != nil {
		t.Fatal(err)
	}
}

func noCountdownFor(t *testing.T, f *rigs.GrillingFixture) {
	t.Helper()
	time.Sleep(rigs.CountdownDelay + 500*time.Millisecond) // negative wait: nothing marks a countdown that must not start
	if got := rigs.SessionSnapshot(t, f); got.AutoQueueAt != nil {
		t.Errorf("auto_queue_at = %v, want no countdown", got.AutoQueueAt)
	}
	if queued(f) || len(f.H.Banners.Sent()) != 0 {
		t.Errorf("queued = %v, banners = %+v, want neither", queued(f), f.H.Banners.Sent())
	}
}

func TestAutoQueueTrigger_ReachingBranchWithTheSwitchOnCountsDownThenQueues(t *testing.T) {
	t.Parallel()
	f, store := startReadyMap(t, mapCase{step: "spec", implStatus: "open", base: "other"})
	setAutoQueue(t, f, true)
	moveMapToBranch(t, f, store)
	if !servertest.Poll(5*time.Second, func() bool { return rigs.SessionSnapshot(t, f).AutoQueueAt != nil }) {
		t.Fatal("no countdown after the map reached Branch")
	}
	if !servertest.Poll(5*time.Second, func() bool { return queued(f) }) {
		t.Fatal("epic not queued at the end of the countdown")
	}
}

func TestAutoQueueTrigger_SwitchNeverTurnedOnQueuesNothing(t *testing.T) {
	t.Parallel()
	f, store := startReadyMap(t, mapCase{step: "spec", implStatus: "open"})
	moveMapToBranch(t, f, store)
	noCountdownFor(t, f)
}

func TestAutoQueueTrigger_AnAlreadyQueuedEpicStartsNoCountdown(t *testing.T) {
	t.Parallel()
	f, store := startReadyMap(t, mapCase{step: "spec", implStatus: "open"})
	setAutoQueue(t, f, true)
	if res, err := f.H.Server.Prompt().QueueEpic("proj", "map-impl"); err != nil || res.Refused {
		t.Fatalf("queue = %+v, %v", res, err)
	}
	moveMapToBranch(t, f, store)
	time.Sleep(rigs.CountdownDelay + 500*time.Millisecond) // negative wait
	if got := rigs.SessionSnapshot(t, f); got.AutoQueueAt != nil {
		t.Errorf("auto_queue_at = %v, want no countdown", got.AutoQueueAt)
	}
	if len(f.H.Banners.Sent()) != 0 {
		t.Errorf("banners = %+v, want none", f.H.Banners.Sent())
	}
}

func TestAutoQueueTrigger_ADraftEpicStartsNoCountdown(t *testing.T) {
	t.Parallel()
	f, store := startReadyMap(t, mapCase{step: "spec", implStatus: "draft"})
	setAutoQueue(t, f, true)
	moveMapToBranch(t, f, store)
	noCountdownFor(t, f)
}

func TestAutoQueueTrigger_ADesignAtBranchWhenTheServerStartsGetsNoCountdown(t *testing.T) {
	t.Parallel()
	f, store := startReadyMap(t, mapCase{step: "branch", implStatus: "draft"})
	setAutoQueue(t, f, true)
	md := "---\nstatus: open\n---\n\n# map-impl\n"
	if err := os.WriteFile(filepath.Join(store, "proj", "map-impl", "ticket.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	f.H.Restart(t)
	noCountdownFor(t, f)
}
