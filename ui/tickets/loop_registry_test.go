package tickets

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

func TestRunSnapshotsAreDeterministicAndIndependent(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(2)
	r.tryStart("epic-b", 2, 4)
	r.tryStart("epic-a", 1, 3)
	r.reduceLiveEvent("epic-b", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-b",
	})
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-a",
	})

	all := r.runSnapshots()
	if len(all) != 2 || all[0].EpicName != "epic-a" || all[1].EpicName != "epic-b" {
		t.Fatalf("runSnapshots() names = %#v, want epic-a then epic-b", all)
	}
	if all[0].Tickets["01"].Label != "iter-a" || all[1].Tickets["01"].Label != "iter-b" {
		t.Fatalf("overlapping ticket identifiers were not isolated: %#v", all)
	}

	one, ok := r.runSnapshot("epic-a")
	if !ok {
		t.Fatal("runSnapshot(epic-a): want snapshot")
	}
	one.Tickets["01"] = RunTicketSnapshot{Label: "mutated"}
	again, _ := r.runSnapshot("epic-a")
	if again.Tickets["01"].Label != "iter-a" {
		t.Fatalf("caller mutation changed registry snapshot: %#v", again.Tickets["01"])
	}
}

func TestLoopRegistryPauseReasons_IndependentAndAnyPaused(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(2)

	if r.isPaused() || r.isSoftLimitPaused() || r.isHardLimitPaused() {
		t.Fatal("expected no pause reason active initially")
	}

	r.pauseSoftLimit()
	if !r.isSoftLimitPaused() {
		t.Fatal("expected pauseSoftLimit to set the soft-limit reason")
	}
	if r.isPaused() || r.isHardLimitPaused() {
		t.Fatal("expected pauseSoftLimit to leave the other reasons untouched")
	}

	r.pause()
	if !r.isPaused() || !r.isSoftLimitPaused() {
		t.Fatal("expected pause() to add its own reason without clearing the soft-limit one")
	}

	r.resumeSoftLimit()
	if r.isSoftLimitPaused() {
		t.Fatal("expected resumeSoftLimit to clear only the soft-limit reason")
	}
	if !r.isPaused() {
		t.Fatal("expected resumeSoftLimit to leave the manual pause untouched")
	}

	r.mu.Lock()
	anyPaused := r.anyPausedLocked()
	r.mu.Unlock()
	if !anyPaused {
		t.Fatal("expected the run to report paused while the manual reason is still active")
	}

	r.resume()
	r.mu.Lock()
	anyPaused = r.anyPausedLocked()
	r.mu.Unlock()
	if anyPaused {
		t.Fatal("expected the run to report unpaused once every reason is cleared")
	}
}

func TestReduceLiveEventCapturesProgressContextAndPause(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 1, 3)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01",
	})
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventContextOccupancy, Identifier: "01", Tokens: 42_000,
	})
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-01",
		PauseKind: ralphloop.PauseNeedsRepair, Reason: "permission required",
	})

	snapshot, ok := r.runSnapshot("epic-a")
	if !ok {
		t.Fatal("runSnapshot(epic-a): want snapshot")
	}
	if snapshot.State != RunStateRunning || snapshot.Done != 1 || snapshot.Total != 3 || snapshot.ContextTokens != 42_000 || !snapshot.Paused {
		t.Fatalf("run snapshot = %#v", snapshot)
	}
	ticket := snapshot.Tickets["01"]
	if !ticket.Paused || ticket.PauseKind != ralphloop.PauseNeedsRepair || ticket.PauseReason != "permission required" || ticket.ContextTokens != 42_000 {
		t.Fatalf("ticket snapshot = %#v", ticket)
	}
}

// TestReduceLiveEventCapturesRunningIterationIdentity is ticket 02's test
// seam: a running iteration's full identity (session id, cwd, agent, pane
// id, tab id) reaches the registry's per-ticket snapshot from
// LiveEventIterationStarted, and is cleared once LiveEventIterationFinished
// lands — the plumbing later cost-aggregation and stop/repair tickets build
// on.
func TestReduceLiveEventCapturesRunningIterationIdentity(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 0, 1)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01",
		SessionID: "sess-1", Cwd: "/repo/iter-01",
		AgentKind: ralphloop.AgentKind("claude"), PaneID: "pane-1", TabID: "tab-1",
	})

	running, ok := r.runSnapshot("epic-a")
	if !ok {
		t.Fatal("runSnapshot(epic-a): want snapshot")
	}
	ticket := running.Tickets["01"]
	if ticket.SessionID != "sess-1" || ticket.Cwd != "/repo/iter-01" ||
		ticket.Agent != ralphloop.AgentKind("claude") || ticket.PaneID != "pane-1" || ticket.TabID != "tab-1" {
		t.Fatalf("running ticket identity = %#v, want full identity", ticket)
	}

	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationFinished, Identifier: "01",
		Stats: ralphloop.IterationStats{Completed: 1, Total: 1},
	})
	finished, _ := r.runSnapshot("epic-a")
	ticket = finished.Tickets["01"]
	if ticket.SessionID != "" || ticket.Cwd != "" || ticket.Agent != "" || ticket.PaneID != "" || ticket.TabID != "" {
		t.Fatalf("finished ticket identity = %#v, want cleared", ticket)
	}
}

func TestReduceLiveEventStampsPerTicketStartedAt(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 0, 2)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01",
	})
	first, _ := r.runSnapshot("epic-a")
	firstStartedAt := first.Tickets["01"].StartedAt
	if firstStartedAt.IsZero() {
		t.Fatal("ticket 01 StartedAt: want non-zero after its own iteration started")
	}

	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "02", Label: "iter-02",
	})
	second, _ := r.runSnapshot("epic-a")
	if second.Tickets["01"].StartedAt != firstStartedAt {
		t.Fatalf("ticket 01 StartedAt changed after a different ticket started: got %v, want %v",
			second.Tickets["01"].StartedAt, firstStartedAt)
	}
	if second.Tickets["02"].StartedAt.IsZero() {
		t.Fatal("ticket 02 StartedAt: want non-zero after its own iteration started")
	}
}

func TestReduceLiveEventCompletesTicketProgress(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 1, 3)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01",
	})
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-01",
	})
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationResumed, Label: "iter-01",
	})
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationFinished, Identifier: "01",
		Stats: ralphloop.IterationStats{Completed: 2, Total: 3},
	})

	snapshot, _ := r.runSnapshot("epic-a")
	if snapshot.Done != 2 || snapshot.Total != 3 || snapshot.Paused || !snapshot.Tickets["01"].Completed {
		t.Fatalf("snapshot after ticket completion = %#v", snapshot)
	}
}

func TestFinishPreservesCompletionAndFailureSnapshots(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(2)
	r.tryStart("epic-a", 1, 1)
	r.tryStart("epic-b", 0, 1)
	r.finish("epic-a", nil)
	r.finish("epic-b", errors.New("agent failed"))
	r.pause()

	succeeded, ok := r.runSnapshot("epic-a")
	if !ok || succeeded.State != RunStateCompleted || succeeded.Paused || succeeded.FinalError != "" {
		t.Fatalf("successful snapshot = %#v, %v", succeeded, ok)
	}
	failed, ok := r.runSnapshot("epic-b")
	if !ok || failed.State != RunStateFailed || failed.FinalError != "agent failed" {
		t.Fatalf("failed snapshot = %#v, %v", failed, ok)
	}
}

func TestRunSnapshotsAllowConcurrentReductionAndReads(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 0, 1)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01",
	})

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(2)
		go func(tokens int) {
			defer wg.Done()
			r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
				Kind: ralphloop.LiveEventContextOccupancy, Identifier: "01", Tokens: tokens,
			})
		}(i)
		go func() {
			defer wg.Done()
			r.runSnapshots()
		}()
	}
	wg.Wait()
	if _, ok := r.runSnapshot("epic-a"); !ok {
		t.Fatal("runSnapshot(epic-a): want snapshot after concurrent access")
	}
}

func TestReduceLiveEventCapturesEpicCompletion(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 2, 3)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventEpicComplete, Completed: 3,
	})

	snapshot, _ := r.runSnapshot("epic-a")
	if snapshot.State != RunStateCompleted {
		t.Fatalf("snapshot after epic completion = %#v", snapshot)
	}
}

// TestReduceLiveEventEpicCompleteDoesNotClobberDiskSyncedDone pins the fix
// for a resumed run reporting stale counts: EpicComplete's Completed is this
// run's own landed-ticket count (see EventSink.EpicComplete), a different
// number from the epic-wide Done a resumed run's IterationFinished events
// already synced — EpicComplete must not overwrite Done with it.
func TestReduceLiveEventEpicCompleteDoesNotClobberDiskSyncedDone(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 5, 10)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "10", Label: "iter-10",
	})
	// A resumed run only landed one ticket itself, but the epic's on-disk
	// state (synced via Stats) already shows all 10 done.
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationFinished, Identifier: "10",
		Stats: ralphloop.IterationStats{Completed: 10, Total: 10},
	})
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventEpicComplete, Completed: 1, ElapsedSeconds: 5,
	})

	snapshot, _ := r.runSnapshot("epic-a")
	if snapshot.Done != 10 || snapshot.Total != 10 {
		t.Fatalf("snapshot.Done/Total = %d/%d, want 10/10 (epic-wide, not this run's landed count of 1)", snapshot.Done, snapshot.Total)
	}
}

func TestDrainPendingNotifyClosesOnTicketReattached(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 0, 2)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventTicketReattached, Identifier: "01", Label: "iter-01",
	})

	ids := r.drainPendingNotifyCloses("epic-a")
	want := reattachNotifyID("epic-a", "01")
	if len(ids) != 1 || ids[0] != want {
		t.Fatalf("drainPendingNotifyCloses() = %#v, want [%q]", ids, want)
	}

	// A second drain finds nothing left to close.
	if ids := r.drainPendingNotifyCloses("epic-a"); len(ids) != 0 {
		t.Fatalf("drainPendingNotifyCloses() after drain = %#v, want empty", ids)
	}
}

func TestDrainPendingNotifyClosesOnIterationResumed(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 0, 2)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01",
	})
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-01",
	})
	if ids := r.drainPendingNotifyCloses("epic-a"); len(ids) != 0 {
		t.Fatalf("drainPendingNotifyCloses() after pause = %#v, want empty (only resume closes)", ids)
	}

	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationResumed, Label: "iter-01",
	})
	ids := r.drainPendingNotifyCloses("epic-a")
	want := reattachNotifyID("epic-a", "01")
	if len(ids) != 1 || ids[0] != want {
		t.Fatalf("drainPendingNotifyCloses() after resume = %#v, want [%q]", ids, want)
	}
}

func TestDrainPendingNotifyClosesOnlyAffectsResumedTicket(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 0, 2)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01",
	})
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventTicketReattached, Identifier: "02", Label: "iter-02",
	})
	r.drainPendingNotifyCloses("epic-a") // clear the reattach-triggered close

	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-01",
	})
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationResumed, Label: "iter-01",
	})

	ids := r.drainPendingNotifyCloses("epic-a")
	want := reattachNotifyID("epic-a", "01")
	if len(ids) != 1 || ids[0] != want {
		t.Fatalf("drainPendingNotifyCloses() = %#v, want only ticket 01's id [%q] (ticket 02 untouched)", ids, want)
	}
}

func TestDrainPendingToastsOnEpicComplete(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 0, 2)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventEpicComplete, EpicName: "epic-a", Completed: 2, ElapsedSeconds: 90,
	})

	toasts := r.drainPendingToasts("epic-a")
	if len(toasts) != 1 || toasts[0].Kind != notify.KindSuccess {
		t.Fatalf("drainPendingToasts() = %#v, want one success toast", toasts)
	}
	if !strings.Contains(toasts[0].Message, "epic-a") || !strings.Contains(toasts[0].Message, "1m30s") {
		t.Fatalf("toast message = %q, want epic name and elapsed time", toasts[0].Message)
	}

	if toasts := r.drainPendingToasts("epic-a"); len(toasts) != 0 {
		t.Fatalf("drainPendingToasts() after drain = %#v, want empty", toasts)
	}
}

func TestReduceLiveEventDrainComplete(t *testing.T) {
	t.Parallel()
	// Resumed/subset run: the epic's true done count (8) already differs
	// from what this run call itself will land (Completed: 1) before the
	// drain even fires — mirrors LiveEventEpicComplete's exclusion.
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 8, 10)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventDrainComplete, EpicName: "epic-a", Completed: 1, ElapsedSeconds: 90,
	})

	snap, ok := r.runSnapshot("epic-a")
	if !ok {
		t.Fatal("runSnapshot() ok = false, want true")
	}
	if snap.State != RunStateCompleted {
		t.Errorf("run.State = %v, want RunStateCompleted", snap.State)
	}
	if snap.Done != 8 {
		t.Errorf("run.Done = %d, want 8 (unchanged by drain-complete's own tally)", snap.Done)
	}

	toasts := r.drainPendingToasts("epic-a")
	if len(toasts) != 1 || toasts[0].Kind != notify.KindWarning {
		t.Fatalf("drainPendingToasts() = %#v, want one warning toast", toasts)
	}
	if !strings.Contains(toasts[0].Message, "epic-a") || !strings.Contains(toasts[0].Message, "1m30s") {
		t.Fatalf("toast message = %q, want epic name and elapsed time", toasts[0].Message)
	}
	if strings.Contains(toasts[0].Message, "complete") {
		t.Fatalf("toast message = %q, wording should be drain-distinct from epic-complete toast", toasts[0].Message)
	}
}

func TestDrainPendingToastsOnNeedsRepairPauseOnly(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 0, 2)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-01",
		PauseKind: ralphloop.PauseRateLimit, Reason: "rate limited",
	})
	if toasts := r.drainPendingToasts("epic-a"); len(toasts) != 0 {
		t.Fatalf("drainPendingToasts() after rate-limit pause = %#v, want empty (needs-repair only)", toasts)
	}

	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-01",
		PauseKind: ralphloop.PauseNeedsRepair, Reason: "permission required",
	})
	toasts := r.drainPendingToasts("epic-a")
	if len(toasts) != 1 || toasts[0].Kind != notify.KindWarning {
		t.Fatalf("drainPendingToasts() after needs-repair pause = %#v, want one warning toast", toasts)
	}
	if !strings.Contains(toasts[0].Message, "iter-01") || !strings.Contains(toasts[0].Message, "permission required") {
		t.Fatalf("toast message = %q, want label and reason", toasts[0].Message)
	}
}

// TestReduceLiveEventSetsPendingReloadOnBothParkStatuses covers the
// reducer's half of the disk-not-event-payload requirement: parking a
// ticket (needs-answer or needs-repair) marks the run for a re-read from
// disk, drained exactly once by drainPendingReload.
func TestReduceLiveEventSetsPendingReloadOnBothParkStatuses(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"needs-answer", "needs-repair"} {
		t.Run(status, func(t *testing.T) {
			r := newLoopRegistry(1)
			r.tryStart("epic-a", 0, 1)
			r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
				Kind: ralphloop.LiveEventTicketNeedsHuman, Identifier: "01", Status: status, Reason: "because",
			})

			if !r.drainPendingReload("epic-a") {
				t.Fatalf("drainPendingReload() = false after %s park, want true", status)
			}
			if r.drainPendingReload("epic-a") {
				t.Fatal("drainPendingReload() a second time = true, want false (drained once)")
			}
		})
	}
}

func TestDrainPendingReloadFalseForUnknownOrUntouchedEpic(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	if r.drainPendingReload("no-such-epic") {
		t.Fatal("drainPendingReload() for unknown epic = true, want false")
	}

	r.tryStart("epic-a", 0, 1)
	if r.drainPendingReload("epic-a") {
		t.Fatal("drainPendingReload() before any park event = true, want false")
	}
}

func TestTryStartSameEpicTwiceFails(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(2)

	if _, ok := r.tryStart("epic-a", 0, 5); !ok {
		t.Fatalf("first tryStart for epic-a: want ok")
	}
	if _, ok := r.tryStart("epic-a", 0, 5); ok {
		t.Fatalf("second tryStart for epic-a while running: want !ok")
	}
}

func TestTryStartDifferentEpicsUpToCapSucceed(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(2)

	if _, ok := r.tryStart("epic-a", 0, 5); !ok {
		t.Fatalf("tryStart epic-a: want ok")
	}
	if _, ok := r.tryStart("epic-b", 0, 3); !ok {
		t.Fatalf("tryStart epic-b while epic-a running and under cap: want ok")
	}
	if !r.isRunning() {
		t.Fatalf("isRunning: want true with two epics running")
	}
}

func TestTryStartBeyondCapIsRefusedSynchronously(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)

	if _, ok := r.tryStart("epic-a", 0, 1); !ok {
		t.Fatal("tryStart epic-a into a free slot: want ok")
	}
	if slots := r.availableSlots(); slots != 0 {
		t.Fatalf("availableSlots after a start reserved the only slot = %d, want 0", slots)
	}
	if _, ok := r.tryStart("epic-b", 0, 1); ok {
		t.Fatal("tryStart epic-b past the cap, before epic-a's run acquired: want !ok")
	}
	if r.isRunningEpic("epic-b") {
		t.Fatal("a refused tryStart recorded a run anyway")
	}
}

func TestFinishReturnsAnUnclaimedReservation(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	if _, ok := r.tryStart("epic-a", 0, 1); !ok {
		t.Fatal("tryStart epic-a: want ok")
	}

	r.finish("epic-a", nil) // the run ended before ever reaching its Acquire

	if slots := r.availableSlots(); slots != 1 {
		t.Fatalf("availableSlots after finishing a never-acquired run = %d, want 1", slots)
	}
	if _, ok := r.tryStart("epic-b", 0, 1); !ok {
		t.Fatal("tryStart epic-b into the freed slot: want ok")
	}
}

func TestFinishTracksEachEpicsErrorIndependently(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(2)

	if _, ok := r.tryStart("epic-a", 0, 5); !ok {
		t.Fatalf("tryStart epic-a: want ok")
	}
	if _, ok := r.tryStart("epic-b", 0, 3); !ok {
		t.Fatalf("tryStart epic-b: want ok")
	}

	wantErr := errors.New("epic-b failed")
	r.finish("epic-a", nil)
	r.finish("epic-b", wantErr)

	if err := r.lastError("epic-a"); err != nil {
		t.Fatalf("lastError(epic-a) = %v, want nil", err)
	}
	if err := r.lastError("epic-b"); !errors.Is(err, wantErr) {
		t.Fatalf("lastError(epic-b) = %v, want %v", err, wantErr)
	}
	if err := r.lastError("epic-b"); !errors.Is(err, wantErr) {
		t.Fatalf("second lastError(epic-b) = %v, want %v", err, wantErr)
	}
}

func TestIsRunningReflectsAnyEpic(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(2)

	if r.isRunning() {
		t.Fatalf("isRunning: want false with nothing started")
	}

	r.tryStart("epic-a", 0, 5)
	if !r.isRunning() {
		t.Fatalf("isRunning: want true with epic-a running")
	}

	r.tryStart("epic-b", 0, 3)
	r.finish("epic-a", nil)
	if !r.isRunning() {
		t.Fatalf("isRunning: want true with epic-b still running")
	}

	r.finish("epic-b", nil)
	if r.isRunning() {
		t.Fatalf("isRunning: want false once every epic has finished")
	}
}

func TestIsRunningEpicReflectsThatEpicOnly(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(2)
	r.tryStart("epic-a", 1, 5)
	r.tryStart("epic-b", 2, 3)

	if !r.isRunningEpic("epic-b") {
		t.Fatal("isRunningEpic(epic-b): want true")
	}

	r.finish("epic-b", nil)
	if r.isRunningEpic("epic-b") {
		t.Fatal("isRunningEpic(epic-b) after it finished: want false")
	}
	if !r.isRunningEpic("epic-a") {
		t.Fatal("isRunningEpic(epic-a) still running: want true")
	}

	r.finish("epic-a", nil)
	if r.isRunningEpic("epic-a") {
		t.Fatal("isRunningEpic(epic-a) after it finished: want false")
	}
}

func TestPauseStopsAllRunGatesAndNewStartsUntilResume(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(2)
	if _, ok := r.tryStart("epic-a", 0, 2); !ok {
		t.Fatal("tryStart epic-a: want ok")
	}
	if _, ok := r.tryStart("epic-b", 0, 2); !ok {
		t.Fatal("tryStart epic-b: want ok")
	}

	r.pause()
	if !r.runs["epic-a"].gate.ForceResume(ralphloop.QueuePauseLabel) || !r.runs["epic-b"].gate.ForceResume(ralphloop.QueuePauseLabel) {
		t.Fatal("pause did not close every running epic's claim gate")
	}
	r.pause()
	if slots := r.availableSlots(); slots != 0 {
		t.Fatalf("availableSlots while paused = %d, want 0", slots)
	}
	if _, ok := r.tryStart("epic-c", 0, 1); ok {
		t.Fatal("tryStart epic-c while paused: want !ok")
	}

	r.resume()
	if r.runs["epic-a"].gate.ForceResume(ralphloop.QueuePauseLabel) || r.runs["epic-b"].gate.ForceResume(ralphloop.QueuePauseLabel) {
		t.Fatal("resume left a running epic's claim gate paused")
	}
}

func TestResumeClearsRateLimitPauses(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(2)
	if _, ok := r.tryStart("epic-a", 0, 2); !ok {
		t.Fatal("tryStart epic-a: want ok")
	}
	run := r.runs["epic-a"]
	run.gate.Pause("iter-01", "rate limit detected, resets 6:49 PM")
	run.gate.Pause("iter-02", "smart zone")
	r.mu.Lock()
	run.tickets["01"] = RunTicketSnapshot{Identifier: "01", Label: "iter-01", Paused: true, PauseKind: ralphloop.PauseRateLimit}
	run.tickets["02"] = RunTicketSnapshot{Identifier: "02", Label: "iter-02", Paused: true, PauseKind: ralphloop.PauseNeedsRepair}
	r.mu.Unlock()

	if got := r.rateLimitPausedCount(); got != 1 {
		t.Fatalf("rateLimitPausedCount = %d, want 1", got)
	}

	r.resume()
	if run.gate.ForceResume("iter-01") {
		t.Fatal("resume left the rate-limit pause on iter-01")
	}
	if !run.gate.ForceResume("iter-02") {
		t.Fatal("resume cleared iter-02, which isn't a rate-limit pause")
	}
}

func TestRegistryDrainsRunEventsBeforeFinish(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(1)
	sink, ok := r.tryStart("epic-a", 0, 1)
	if !ok {
		t.Fatal("tryStart epic-a: want ok")
	}
	sink.IterationStarted(tickets.Ticket{Identifier: "01"}, "iter-01", "", "", "", "", "")
	sink.ContextOccupancy("01", 42)
	sink.IterationFinished(tickets.Ticket{Identifier: "01"}, "epic-a", ralphloop.IterationStats{Completed: 1, Total: 1})

	r.finish("epic-a", nil)

	snapshot, ok := r.runSnapshot("epic-a")
	if !ok || snapshot.Done != 1 || !snapshot.Tickets["01"].Completed {
		t.Fatalf("snapshot after finish = %#v, %v", snapshot, ok)
	}
	if r.snapshots["epic-a"].sink != nil {
		t.Fatal("finish retained event sink")
	}
}

func TestRegistryDrainsEpicsIndependently(t *testing.T) {
	t.Parallel()
	r := newLoopRegistry(2)
	sinkA, _ := r.tryStart("epic-a", 0, 1)
	sinkB, _ := r.tryStart("epic-b", 0, 1)
	sinkA.IterationStarted(tickets.Ticket{Identifier: "01"}, "iter-a", "", "", "", "", "")
	sinkB.IterationStarted(tickets.Ticket{Identifier: "01"}, "iter-b", "", "", "", "", "")
	sinkA.ContextOccupancy("01", 11)
	sinkB.ContextOccupancy("01", 22)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r.finish("epic-a", nil)
	}()
	go func() {
		defer wg.Done()
		r.finish("epic-b", nil)
	}()
	wg.Wait()

	snapshotA, _ := r.runSnapshot("epic-a")
	snapshotB, _ := r.runSnapshot("epic-b")
	if snapshotA.Tickets["01"].Label != "iter-a" || snapshotA.ContextTokens != 11 {
		t.Fatalf("epic-a snapshot = %#v", snapshotA)
	}
	if snapshotB.Tickets["01"].Label != "iter-b" || snapshotB.ContextTokens != 22 {
		t.Fatalf("epic-b snapshot = %#v", snapshotB)
	}
}
