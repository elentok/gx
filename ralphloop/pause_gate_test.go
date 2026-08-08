package ralphloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPauseGate_NotPausedInitially(t *testing.T) {
	t.Parallel()
	g := NewGate()
	if g.isPaused() {
		t.Error("isPaused() = true for a fresh gate, want false")
	}
}

func TestPauseGate_PauseMarksPausedAndRecordsReason(t *testing.T) {
	t.Parallel()
	g := NewGate()
	g.pause("iter-01", "context occupancy breach")

	if !g.isPaused() {
		t.Error("isPaused() = false after pause(), want true")
	}
	if got := g.snapshot()["iter-01"]; got != "context occupancy breach" {
		t.Errorf(`snapshot()["iter-01"] = %q, want "context occupancy breach"`, got)
	}
}

func TestPauseGate_Drain_RefusesFutureClaimsWithoutRunningTheClaimFunc(t *testing.T) {
	g := NewGate()
	g.Drain()

	if !g.isDraining() {
		t.Error("isDraining() = false after Drain(), want true")
	}

	ran := false
	admitted, err := g.claimIfRunning(func() error {
		ran = true
		return nil
	})
	if err != nil {
		t.Fatalf("claimIfRunning() error = %v", err)
	}
	if admitted {
		t.Error("claimIfRunning() admitted = true after Drain(), want false")
	}
	if ran {
		t.Error("claimIfRunning() ran the claim func after Drain(), want it skipped")
	}
}

func TestPauseGate_MultiplePausedIterations_AllStayPausedUntilForceResumeClearsLast(t *testing.T) {
	t.Parallel()
	g := NewGate()
	g.pause("iter-01", "breach one")
	g.pause("iter-02", "breach two")

	if got := g.snapshot(); len(got) != 2 {
		t.Fatalf("snapshot() = %v, want both iterations recorded as paused", got)
	}

	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			g.waitForResume(context.Background())
		})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("waitForResume() returned before ForceResume cleared either label")
	case <-time.After(50 * time.Millisecond):
	}

	g.ForceResume("iter-01")
	select {
	case <-done:
		t.Fatal("waitForResume() returned while iter-02 is still paused")
	case <-time.After(50 * time.Millisecond):
	}

	g.ForceResume("iter-02")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() never returned for both waiters")
	}

	if g.isPaused() {
		t.Error("isPaused() = true after resume, want false")
	}
}

func TestPauseGate_ForceResume_WakesWaiterImmediately(t *testing.T) {
	t.Parallel()
	g := NewGate()
	g.pause("iter-01", "breach")

	returned := make(chan struct{})
	go func() {
		g.waitForResume(context.Background())
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("waitForResume() returned before ForceResume")
	case <-time.After(50 * time.Millisecond):
	}

	if wasPaused := g.ForceResume("iter-01"); !wasPaused {
		t.Fatal("ForceResume(\"iter-01\") = false, want true: iter-01 was paused")
	}

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() did not return after ForceResume")
	}

	if g.isPaused() {
		t.Error("isPaused() = true after ForceResume, want false")
	}
}

func TestPauseGate_ForceResume_UnknownLabelReturnsFalse(t *testing.T) {
	t.Parallel()
	g := NewGate()
	g.pause("iter-01", "breach")

	if g.ForceResume("iter-02") {
		t.Error("ForceResume(unpaused label) = true, want false")
	}
	if !g.isPaused() {
		t.Error("isPaused() = false after ForceResume of an unrelated label, want true: iter-01 is still paused")
	}
}

// TestPauseGate_ForceResumeBeforePause_WaitForResumeReturnsImmediately covers
// the race where ForceResume lands in the gap between a caller's pause() and
// its own subsequent waitForResume() call (e.g. a TUI operator resuming the
// instant it observes the pause). Without the len(reasons)==0 short-circuit
// in waitForResume, that late arrival would make it the new leader for a
// pause that no longer exists, polling forever for a resume signal that
// already happened.
func TestPauseGate_ForceResumeBeforePause_WaitForResumeReturnsImmediately(t *testing.T) {
	t.Parallel()
	g := NewGate()
	g.pause("iter-01", "breach")

	if !g.ForceResume("iter-01") {
		t.Fatal("ForceResume(iter-01) = false, want true")
	}

	returned := make(chan struct{})
	go func() {
		g.waitForResume(context.Background())
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() never returned even though the label was never actually paused")
	}
}
