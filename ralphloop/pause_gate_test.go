package ralphloop

import (
	"sync"
	"testing"
	"time"
)

func TestPauseGate_NotPausedInitially(t *testing.T) {
	g := NewGate()
	if g.isPaused() {
		t.Error("isPaused() = true for a fresh gate, want false")
	}
}

func TestPauseGate_PauseMarksPausedAndRecordsReason(t *testing.T) {
	g := NewGate()
	g.pause("iter-01", "context occupancy breach")

	if !g.isPaused() {
		t.Error("isPaused() = false after pause(), want true")
	}
	if got := g.snapshot()["iter-01"]; got != "context occupancy breach" {
		t.Errorf(`snapshot()["iter-01"] = %q, want "context occupancy breach"`, got)
	}
}

func TestPauseGate_MultiplePausedIterations_AllStayPausedUntilOneResumeSignal(t *testing.T) {
	g := NewGate()
	g.pause("iter-01", "breach one")
	g.pause("iter-02", "breach two")

	if got := g.snapshot(); len(got) != 2 {
		t.Fatalf("snapshot() = %v, want both iterations recorded as paused", got)
	}

	d := Deps{
		ResumeSignaled: func(path string) (bool, error) { return true, nil },
		Sleep:          func(time.Duration) {},
	}

	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			g.waitForResume(d, "unused")
		})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() never returned for both waiters")
	}

	if g.isPaused() {
		t.Error("isPaused() = true after resume, want false")
	}
}

func TestPauseGate_WaitForResume_BlocksUntilSignaled(t *testing.T) {
	g := NewGate()
	g.pause("iter-01", "breach")

	var mu sync.Mutex
	signaled := false
	d := Deps{
		ResumeSignaled: func(path string) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			return signaled, nil
		},
		Sleep: func(time.Duration) {},
	}

	returned := make(chan struct{})
	go func() {
		g.waitForResume(d, "unused")
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("waitForResume() returned before the signal was set")
	case <-time.After(50 * time.Millisecond):
	}

	mu.Lock()
	signaled = true
	mu.Unlock()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() never returned after the signal was set")
	}
}

// TestPauseGate_ForceResume_WakesWaiterWithoutWaitingForSleep is the
// in-process control path's whole point: unlike a `gx ralph-loop resume`
// file signal, which the leader only notices on its next resumePollInterval
// tick, ForceResume must release a waiter immediately — even mid-tick, with
// Sleep still blocked and ResumeSignaled still reporting false.
func TestPauseGate_ForceResume_WakesWaiterWithoutWaitingForSleep(t *testing.T) {
	g := NewGate()
	g.pause("iter-01", "breach")

	sleepStarted := make(chan struct{})
	sleepBlock := make(chan struct{})
	d := Deps{
		ResumeSignaled: func(path string) (bool, error) { return false, nil },
		Sleep: func(time.Duration) {
			close(sleepStarted)
			<-sleepBlock // never closed here: a real poll tick would still be blocked
		},
	}

	returned := make(chan struct{})
	go func() {
		g.waitForResume(d, "unused")
		close(returned)
	}()

	select {
	case <-sleepStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() never reached its poll-interval sleep")
	}

	if wasPaused := g.ForceResume("iter-01"); !wasPaused {
		t.Fatal("ForceResume(\"iter-01\") = false, want true: iter-01 was paused")
	}

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() did not return after ForceResume, even with Sleep still blocked")
	}

	if g.isPaused() {
		t.Error("isPaused() = true after ForceResume, want false")
	}
}

func TestPauseGate_ForceResume_UnknownLabelReturnsFalse(t *testing.T) {
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
	g := NewGate()
	g.pause("iter-01", "breach")

	if !g.ForceResume("iter-01") {
		t.Fatal("ForceResume(iter-01) = false, want true")
	}

	d := Deps{
		ResumeSignaled: func(path string) (bool, error) { return false, nil },
		Sleep:          func(time.Duration) {},
	}

	returned := make(chan struct{})
	go func() {
		g.waitForResume(d, "unused")
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForResume() never returned even though the label was never actually paused")
	}
}
