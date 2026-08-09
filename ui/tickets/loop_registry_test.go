package tickets

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

func TestRunSnapshotsAreDeterministicAndIndependent(t *testing.T) {
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

func TestLoopRegistryConcurrencyCanBeReconfigured(t *testing.T) {
	r := newLoopRegistry(2)
	r.Acquire()
	r.Acquire()

	r.setMaxConcurrent(1)
	blocked := make(chan struct{})
	go func() {
		r.Acquire()
		close(blocked)
	}()
	select {
	case <-blocked:
		t.Fatal("Acquire above reconfigured cap: want it to block")
	case <-time.After(20 * time.Millisecond):
	}

	r.setMaxConcurrent(3)
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("Acquire after raising cap: want it to unblock")
	}
}

func TestReduceLiveEventCapturesProgressContextAndPause(t *testing.T) {
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
		PauseKind: ralphloop.PauseNeedsAttention, Reason: "permission required",
	})

	snapshot, ok := r.runSnapshot("epic-a")
	if !ok {
		t.Fatal("runSnapshot(epic-a): want snapshot")
	}
	if snapshot.State != RunStateRunning || snapshot.Done != 1 || snapshot.Total != 3 || snapshot.ContextTokens != 42_000 || !snapshot.Paused {
		t.Fatalf("run snapshot = %#v", snapshot)
	}
	ticket := snapshot.Tickets["01"]
	if !ticket.Paused || ticket.PauseKind != ralphloop.PauseNeedsAttention || ticket.PauseReason != "permission required" || ticket.ContextTokens != 42_000 {
		t.Fatalf("ticket snapshot = %#v", ticket)
	}
}

func TestReduceLiveEventStampsPerTicketStartedAt(t *testing.T) {
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
	})

	snapshot, _ := r.runSnapshot("epic-a")
	if snapshot.Done != 2 || snapshot.Paused || !snapshot.Tickets["01"].Completed {
		t.Fatalf("snapshot after ticket completion = %#v", snapshot)
	}
}

func TestFinishPreservesCompletionAndFailureSnapshots(t *testing.T) {
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
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 2, 3)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventEpicComplete, Completed: 3,
	})

	snapshot, _ := r.runSnapshot("epic-a")
	if snapshot.State != RunStateCompleted || snapshot.Done != 3 {
		t.Fatalf("snapshot after epic completion = %#v", snapshot)
	}
}

func TestDrainPendingNotifyClosesOnTicketReattached(t *testing.T) {
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

func TestDrainPendingToastsOnNeedsAttentionPauseOnly(t *testing.T) {
	r := newLoopRegistry(1)
	r.tryStart("epic-a", 0, 2)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-01",
		PauseKind: ralphloop.PauseRateLimit, Reason: "rate limited",
	})
	if toasts := r.drainPendingToasts("epic-a"); len(toasts) != 0 {
		t.Fatalf("drainPendingToasts() after rate-limit pause = %#v, want empty (needs-attention only)", toasts)
	}

	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-01",
		PauseKind: ralphloop.PauseNeedsAttention, Reason: "permission required",
	})
	toasts := r.drainPendingToasts("epic-a")
	if len(toasts) != 1 || toasts[0].Kind != notify.KindWarning {
		t.Fatalf("drainPendingToasts() after needs-attention pause = %#v, want one warning toast", toasts)
	}
	if !strings.Contains(toasts[0].Message, "iter-01") || !strings.Contains(toasts[0].Message, "permission required") {
		t.Fatalf("toast message = %q, want label and reason", toasts[0].Message)
	}
}

func TestTryStartSameEpicTwiceFails(t *testing.T) {
	r := newLoopRegistry(2)

	if _, ok := r.tryStart("epic-a", 0, 5); !ok {
		t.Fatalf("first tryStart for epic-a: want ok")
	}
	if _, ok := r.tryStart("epic-a", 0, 5); ok {
		t.Fatalf("second tryStart for epic-a while running: want !ok")
	}
}

func TestTryStartDifferentEpicsUpToCapSucceed(t *testing.T) {
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

func TestTryStartBeyondCapFailsUntilSlotFrees(t *testing.T) {
	r := newLoopRegistry(2)

	r.Acquire()
	r.Acquire()

	blocked := make(chan struct{})
	go func() {
		r.Acquire()
		close(blocked)
	}()
	select {
	case <-blocked:
		t.Fatalf("Acquire beyond cap: want it to block")
	case <-time.After(20 * time.Millisecond):
	}

	r.Release()

	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatalf("Acquire after a slot freed: want it to unblock")
	}
}

func TestParkedEpicDoesNotCountTowardCap(t *testing.T) {
	r := newLoopRegistry(1)

	r.Acquire()
	if slots := r.availableSlots(); slots != 0 {
		t.Fatalf("availableSlots while holding the only permit = %d, want 0", slots)
	}

	r.Release() // simulates a park: the epic is still in r.runs, but no longer holds a permit
	if slots := r.availableSlots(); slots != 1 {
		t.Fatalf("availableSlots after releasing (parking) = %d, want 1", slots)
	}

	if _, ok := r.tryStart("epic-other", 0, 1); !ok {
		t.Fatal("tryStart for a different epic: want ok, tryStart no longer cap-gates")
	}
}

func TestQueuedEpicStartsWhenRunningEpicParks(t *testing.T) {
	r := newLoopRegistry(1)

	r.Acquire()

	blocked := make(chan struct{})
	go func() {
		r.Acquire()
		close(blocked)
	}()
	select {
	case <-blocked:
		t.Fatal("second Acquire at cap 1: want it to block")
	case <-time.After(20 * time.Millisecond):
	}

	r.Release() // first epic parks, freeing its permit

	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("second Acquire after first Release (park): want it to unblock")
	}
}

func TestResumingEpicWaitsForPermitWhenCapIsFullThenProceeds(t *testing.T) {
	r := newLoopRegistry(1)

	r.Acquire()
	r.Release() // "park"

	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		r.Acquire()
		close(firstDone)
	}()
	go func() {
		r.Acquire()
		close(secondDone)
	}()

	var proceeded, blocked chan struct{}
	select {
	case <-firstDone:
		proceeded, blocked = firstDone, secondDone
	case <-secondDone:
		proceeded, blocked = secondDone, firstDone
	case <-time.After(time.Second):
		t.Fatal("neither resuming epic's Acquire proceeded")
	}
	_ = proceeded

	select {
	case <-blocked:
		t.Fatal("the second resuming epic's Acquire: want it still blocked")
	case <-time.After(20 * time.Millisecond):
	}

	r.Release()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("the second resuming epic's Acquire after Release: want it to unblock")
	}
}

func TestFinishTracksEachEpicsErrorIndependently(t *testing.T) {
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

func TestRegistryDrainsRunEventsBeforeFinish(t *testing.T) {
	r := newLoopRegistry(1)
	sink, ok := r.tryStart("epic-a", 0, 1)
	if !ok {
		t.Fatal("tryStart epic-a: want ok")
	}
	sink.IterationStarted("01", "iter-01", "", "")
	sink.ContextOccupancy("01", 42)
	sink.IterationFinished(tickets.Ticket{Identifier: "01"}, "epic-a", ralphloop.IterationStats{})

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
	r := newLoopRegistry(2)
	sinkA, _ := r.tryStart("epic-a", 0, 1)
	sinkB, _ := r.tryStart("epic-b", 0, 1)
	sinkA.IterationStarted("01", "iter-a", "", "")
	sinkB.IterationStarted("01", "iter-b", "", "")
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
