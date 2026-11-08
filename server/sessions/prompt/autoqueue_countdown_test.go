package prompt_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// startReadyMap starts a Design session on proj:map with its impl epic
// ready to be queued, and a countdown delay set.
func startReadyMap(t *testing.T, c mapCase) (*servertest.GrillingFixture, string) {
	t.Helper()
	store := t.TempDir()
	c.name = "map"
	writeMapCase(t, store, c)
	return servertest.StartAdvancing(t, store), store
}

func setAutoQueue(t *testing.T, f *servertest.GrillingFixture, on bool) {
	t.Helper()
	if res, err := f.H.Client.Sessions.SetAutoQueue(f.Ctx, f.ID, on); err != nil || res.Refused {
		t.Fatalf("set auto-queue %v = %+v, %v", on, res, err)
	}
}

func queued(f *servertest.GrillingFixture) bool {
	return strings.Contains(queueText(f), "map-impl")
}

func queueText(f *servertest.GrillingFixture) string {
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

func waitForBanner(t *testing.T, f *servertest.GrillingFixture, want string) {
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
	got := servertest.SessionSnapshot(t, f)
	if got.AutoQueueAt == nil || got.AutoQueueAt.Before(before.Add(servertest.CountdownDelay-time.Millisecond)) || got.AutoQueueAt.After(time.Now().Add(servertest.CountdownDelay)) {
		t.Fatalf("auto_queue_at = %v, want about %v after the switch", got.AutoQueueAt, servertest.CountdownDelay)
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

func TestAutoQueue_SwitchOnOverANotReadyMapStartsNothing(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]mapCase{
		"step":  {step: "spec", implStatus: "open"},
		"draft": {step: "branch", implStatus: "draft"},
		"done":  {step: "branch", implStatus: "done"},
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := startReadyMap(t, c)
			setAutoQueue(t, f, true)
			if got := servertest.SessionSnapshot(t, f); got.AutoQueueAt != nil || !got.AutoQueue {
				t.Errorf("session = switch %v at %v, want the switch on with no countdown", got.AutoQueue, got.AutoQueueAt)
			}
			time.Sleep(servertest.CountdownDelay + 500*time.Millisecond)
			if queued(f) || len(f.H.Banners.Sent()) != 0 {
				t.Errorf("queued = %v, banners = %+v, want neither", queued(f), f.H.Banners.Sent())
			}
		})
	}
}

func TestAutoQueue_SwitchOnOverAHeldOrQueuedMapStartsNothing(t *testing.T) {
	t.Parallel()
	f, _ := startReadyMap(t, mapCase{step: "branch", implStatus: "open"})
	if err := f.H.Server.Workspaces().Hold("proj:map"); err != nil {
		t.Fatal(err)
	}
	setAutoQueue(t, f, true)
	if got := servertest.SessionSnapshot(t, f); got.AutoQueueAt != nil {
		t.Errorf("auto_queue_at = %v over a held design, want none", got.AutoQueueAt)
	}
	if len(f.H.Banners.Sent()) != 0 {
		t.Errorf("banners = %+v, want none", f.H.Banners.Sent())
	}
}

func TestAutoQueue_ConditionsFailingDuringTheCountdownQueueNothingAndSendABanner(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		change func(t *testing.T, f *servertest.GrillingFixture, store string)
		want   string
	}{
		"draft": {want: "is a draft", change: func(t *testing.T, _ *servertest.GrillingFixture, store string) {
			md := "---\nstatus: draft\n---\n\n# map-impl\n"
			if err := os.WriteFile(filepath.Join(store, "proj", "map-impl", "ticket.md"), []byte(md), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		"queued by hand": {want: "already queued", change: func(t *testing.T, f *servertest.GrillingFixture, _ string) {
			if res, err := f.H.Server.Prompt().QueueEpic("proj", "map-impl"); err != nil || res.Refused {
				t.Fatalf("queue = %+v, %v", res, err)
			}
		}},
		"design held": {want: "held", change: func(t *testing.T, f *servertest.GrillingFixture, _ string) {
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

func TestAutoQueue_SwitchOffCancelsTheCountdownWithABanner(t *testing.T) {
	t.Parallel()
	f, _ := startReadyMap(t, mapCase{step: "branch", implStatus: "open"})
	setAutoQueue(t, f, true)
	if servertest.SessionSnapshot(t, f).AutoQueueAt == nil {
		t.Fatal("no countdown running")
	}
	setAutoQueue(t, f, false)
	if got := servertest.SessionSnapshot(t, f); got.AutoQueueAt != nil || got.AutoQueue {
		t.Errorf("session = switch %v at %v, want off with no countdown", got.AutoQueue, got.AutoQueueAt)
	}
	waitForBanner(t, f, "Auto-queue switched off")
	time.Sleep(servertest.CountdownDelay + 500*time.Millisecond)
	if queued(f) {
		t.Error("queued after the switch went off")
	}
}

func TestAutoQueue_SwitchOffLeavesAutoAdvanceCountingDown(t *testing.T) {
	t.Parallel()
	store := servertest.StoreWithMapStep(t, "spec")
	f := servertest.StartAdvancing(t, store)
	writeAssistantMessage(t, f.Path, runEndText)
	endTurn(t, f)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	servertest.SessionSnapshot(t, f)
	setAutoQueue(t, f, true)
	setAutoQueue(t, f, false)
	got := servertest.SessionSnapshot(t, f)
	if got.AutoAdvanceAt == nil {
		t.Errorf("auto_advance_at cleared by switching auto-queue off: %+v", got)
	}
}

func TestAutoQueue_ARestartDropsTheCountdownAndKeepsTheSwitch(t *testing.T) {
	t.Parallel()
	f, _ := startReadyMap(t, mapCase{step: "branch", implStatus: "open"})
	setAutoQueue(t, f, true)
	f.H.Restart(t)
	got := servertest.SessionSnapshot(t, f)
	if !got.AutoQueue || got.AutoQueueAt != nil {
		t.Errorf("after restart switch = %v at %v, want on with no countdown", got.AutoQueue, got.AutoQueueAt)
	}
}
