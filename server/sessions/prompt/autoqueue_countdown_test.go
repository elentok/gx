package prompt_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

// startReadyMap starts a Design session on proj:map with its impl epic
// ready to be queued, and a countdown delay set.
func startReadyMap(t *testing.T, c mapCase) (*rigs.GrillingFixture, string) {
	t.Helper()
	store := t.TempDir()
	c.name = "map"
	writeMapCase(t, store, c)
	return rigs.StartAdvancing(t, store), store
}

func setAutoQueue(t *testing.T, f *rigs.GrillingFixture, on bool) {
	t.Helper()
	if res, err := f.H.Client.Sessions.SetAutoQueue(f.Ctx, f.ID, on); err != nil || res.Refused {
		t.Fatalf("set auto-queue %v = %+v, %v", on, res, err)
	}
}

func queued(f *rigs.GrillingFixture) bool {
	return strings.Contains(queueText(f), "map-impl")
}

func queueText(f *rigs.GrillingFixture) string {
	items, err := f.H.Client.Queue.Items(f.Ctx)
	if err != nil {
		return ""
	}
	var addrs []string
	for _, it := range items {
		addrs = append(addrs, it.Address)
	}
	return strings.Join(addrs, "\n")
}

func waitForBanner(t *testing.T, f *rigs.GrillingFixture, want string) {
	t.Helper()
	if !servertest.Poll(5*time.Second, func() bool {
		for _, b := range f.H.Banners.Sent() {
			if strings.Contains(b.Body+b.Title, want) {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("no banner containing %q, got %+v", want, f.H.Banners.Sent())
	}
}

func TestAutoQueue_SwitchOnOverAReadyMapCountsDownThenQueues(t *testing.T) {
	t.Parallel()
	f, _ := startReadyMap(t, mapCase{step: "branch", implStatus: "open", base: "other"})
	before := time.Now()
	setAutoQueue(t, f, true)
	got := rigs.SessionSnapshot(t, f)
	if got.AutoQueueAt == nil || got.AutoQueueAt.Before(before.Add(rigs.CountdownDelay-time.Millisecond)) || got.AutoQueueAt.After(time.Now().Add(rigs.CountdownDelay)) {
		t.Fatalf("auto_queue_at = %v, want about %v after the switch", got.AutoQueueAt, rigs.CountdownDelay)
	}
	if queued(f) {
		t.Fatal("queued before the countdown ended")
	}
	servertest.WaitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoQueueAt == nil })
	if !servertest.Poll(5*time.Second, func() bool { return queued(f) }) {
		t.Fatal("epic not queued at the end of the countdown")
	}
	if b := f.H.Banners.Sent(); len(b) != 0 {
		t.Errorf("banners = %+v, want none after a successful queue", b)
	}
}

func TestAutoQueue_SwitchOnOverANotReadyOrHeldMapStartsNothing(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		mapCase
		hold bool
	}{
		"step":  {mapCase: mapCase{step: "spec", implStatus: "open"}},
		"draft": {mapCase: mapCase{step: "branch", implStatus: "draft"}},
		"done":  {mapCase: mapCase{step: "branch", implStatus: "done"}},
		"held":  {mapCase: mapCase{step: "branch", implStatus: "open"}, hold: true},
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := startReadyMap(t, c.mapCase)
			if c.hold {
				if err := f.H.Server.Workspaces().Hold("proj:map"); err != nil {
					t.Fatal(err)
				}
			}
			setAutoQueue(t, f, true)
			if got := rigs.SessionSnapshot(t, f); got.AutoQueueAt != nil || !got.AutoQueue {
				t.Errorf("session = switch %v at %v, want the switch on with no countdown", got.AutoQueue, got.AutoQueueAt)
			}
			time.Sleep(rigs.CountdownDelay + 500*time.Millisecond)
			if queued(f) || len(f.H.Banners.Sent()) != 0 {
				t.Errorf("queued = %v, banners = %+v, want neither", queued(f), f.H.Banners.Sent())
			}
		})
	}
}

func TestAutoQueue_ConditionsFailingDuringTheCountdownQueueNothingAndSendABanner(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		change func(t *testing.T, f *rigs.GrillingFixture, store string)
		want   string
	}{
		"draft": {want: "is a draft", change: func(t *testing.T, _ *rigs.GrillingFixture, store string) {
			md := "---\nstatus: draft\n---\n\n# map-impl\n"
			if err := os.WriteFile(filepath.Join(store, "proj", "map-impl", "ticket.md"), []byte(md), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		"queued by hand": {want: "already queued", change: func(t *testing.T, f *rigs.GrillingFixture, _ string) {
			if res, err := f.H.Server.Prompt().QueueEpic("proj", "map-impl"); err != nil || res.Refused {
				t.Fatalf("queue = %+v, %v", res, err)
			}
		}},
		"design held": {want: "held", change: func(t *testing.T, f *rigs.GrillingFixture, _ string) {
			if err := f.H.Server.Workspaces().Hold("proj:map"); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(name, func(t *testing.T) {
			f, store := startReadyMap(t, mapCase{step: "branch", implStatus: "open"})
			setAutoQueue(t, f, true)
			c.change(t, f, store)
			waitForBanner(t, f, c.want)
			if name != "queued by hand" && queued(f) {
				t.Error("queued despite the failing condition")
			}
			if n := strings.Count(queueText(f), "map-impl"); n > 1 {
				t.Errorf("queue holds map-impl %d times", n)
			}
		})
	}
}

func TestAutoQueue_SwitchOffOrCancelStopsTheCountdownWithABanner(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		stop       func(t *testing.T, f *rigs.GrillingFixture)
		wantSwitch bool
		banner     string
	}{
		"switch off": {
			stop:   func(t *testing.T, f *rigs.GrillingFixture) { setAutoQueue(t, f, false) },
			banner: "Auto-queue switched off",
		},
		"cancel": {
			stop: func(t *testing.T, f *rigs.GrillingFixture) {
				if res, err := f.H.Client.Sessions.CancelAutoQueue(f.Ctx, f.ID); err != nil || res.Refused {
					t.Fatalf("cancel = %+v, %v", res, err)
				}
			},
			wantSwitch: true,
			banner:     "Auto-queue cancelled",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, _ := startReadyMap(t, mapCase{step: "branch", implStatus: "open"})
			setAutoQueue(t, f, true)
			if rigs.SessionSnapshot(t, f).AutoQueueAt == nil {
				t.Fatal("no countdown running")
			}
			c.stop(t, f)
			if got := rigs.SessionSnapshot(t, f); got.AutoQueueAt != nil || got.AutoQueue != c.wantSwitch {
				t.Errorf("session = switch %v at %v, want switch %v with no countdown", got.AutoQueue, got.AutoQueueAt, c.wantSwitch)
			}
			waitForBanner(t, f, c.banner)
			time.Sleep(rigs.CountdownDelay + 500*time.Millisecond)
			if queued(f) {
				t.Error("queued after the countdown was stopped")
			}
		})
	}
}

func TestAutoQueue_CancelWithNoCountdownIsANoOpThatReturnsTheSession(t *testing.T) {
	t.Parallel()
	f, _ := startReadyMap(t, mapCase{step: "branch", implStatus: "open"})
	res, err := f.H.Client.Sessions.CancelAutoQueue(f.Ctx, f.ID)
	if err != nil || res.Refused || res.Session == nil || res.Session.ID != f.ID {
		t.Fatalf("cancel = %+v, %v, want the session", res, err)
	}
	for _, b := range f.H.Banners.Sent() {
		if strings.Contains(b.Body+b.Title, "Auto-queue cancelled") {
			t.Errorf("banner %+v sent with no countdown", b)
		}
	}
}

func TestAutoQueue_CancelLeavesAutoAdvanceAndAutoAdvanceCancelLeavesAutoQueue(t *testing.T) {
	t.Parallel()
	f, store := startReadyMap(t, mapCase{step: "spec", implStatus: "open"})
	writeAssistantMessage(t, f.Path, runEndText)
	endTurn(t, f)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	setAutoQueue(t, f, true)
	writeMapCase(t, store, mapCase{name: "map", step: "branch", implStatus: "open"})
	prompt := f.H.Server.Prompt()
	if !servertest.Poll(5*time.Second, func() bool { reason, err := prompt.AutoQueueNotReady("proj", "map"); return err == nil && reason == "" }) {
		t.Fatal("the map never became ready")
	}
	prompt.DesignReachedBranch("proj", "map")
	if got := rigs.SessionSnapshot(t, f); got.AutoAdvanceAt == nil || got.AutoQueueAt == nil {
		t.Fatalf("countdowns: advance at %v, queue at %v, want both running", got.AutoAdvanceAt, got.AutoQueueAt)
	}

	if res, err := f.H.Client.Sessions.CancelAutoQueue(f.Ctx, f.ID); err != nil || res.Refused {
		t.Fatalf("cancel auto-queue = %+v, %v", res, err)
	}
	got := rigs.SessionSnapshot(t, f)
	if got.AutoQueueAt != nil || got.AutoAdvanceAt == nil {
		t.Fatalf("after cancelling auto-queue: queue at %v, advance at %v, want only advance running", got.AutoQueueAt, got.AutoAdvanceAt)
	}

	setAutoQueue(t, f, false)
	setAutoQueue(t, f, true)
	if rigs.SessionSnapshot(t, f).AutoQueueAt == nil {
		t.Fatal("no auto-queue countdown after switching it back on")
	}
	if res, err := f.H.Client.Sessions.CancelAutoAdvance(f.Ctx, f.ID); err != nil || res.Refused {
		t.Fatalf("cancel auto-advance = %+v, %v", res, err)
	}
	got = rigs.SessionSnapshot(t, f)
	if got.AutoAdvanceAt != nil || got.AutoQueueAt == nil {
		t.Errorf("after cancelling auto-advance: queue at %v, advance at %v, want only queue running", got.AutoQueueAt, got.AutoAdvanceAt)
	}
}

func TestAutoQueue_SwitchOffLeavesAutoAdvanceCountingDown(t *testing.T) {
	t.Parallel()
	store := rigs.StoreWithMapStep(t, "spec")
	f := rigs.StartAdvancing(t, store)
	writeAssistantMessage(t, f.Path, runEndText)
	endTurn(t, f)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	rigs.SessionSnapshot(t, f)
	setAutoQueue(t, f, true)
	setAutoQueue(t, f, false)
	got := rigs.SessionSnapshot(t, f)
	if got.AutoAdvanceAt == nil {
		t.Errorf("auto_advance_at cleared by switching auto-queue off: %+v", got)
	}
}

func TestAutoQueue_ARestartDropsTheCountdownAndKeepsTheSwitch(t *testing.T) {
	t.Parallel()
	f, _ := startReadyMap(t, mapCase{step: "branch", implStatus: "open"})
	setAutoQueue(t, f, true)
	f.H.Restart(t)
	got := rigs.SessionSnapshot(t, f)
	if !got.AutoQueue || got.AutoQueueAt != nil {
		t.Errorf("after restart switch = %v at %v, want on with no countdown", got.AutoQueue, got.AutoQueueAt)
	}
}
