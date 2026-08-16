package ralphloop

import (
	"context"
	"maps"
	"sync"
	"time"
)

// parkPollInterval is how often a parked run (nothing runnable, something
// human-clearable) re-reads the epic looking for a status a person has
// cleared. Slower than resumePollInterval (see ratelimit.go) on purpose:
// every pass writes a scheduler-scan line to the run log, and a park can
// last hours.
const parkPollInterval = 30 * time.Second

// QueuePauseLabel identifies the in-process pause controlled by the Queue UI.
const QueuePauseLabel = "queue"

// BudgetPauseLabel identifies the in-process pause controlled by the
// soft-limit budget check — a separate label from QueuePauseLabel so a
// budget pause and a manual pause never clear each other via ForceResume.
const BudgetPauseLabel = "budget"

// BudgetHardPauseLabel identifies the in-process pause controlled by the
// hard-limit budget kill — a separate label from both QueuePauseLabel and
// BudgetPauseLabel so a hard-limit pause, a soft-limit pause, and a manual
// pause never clear each other via ForceResume.
const BudgetHardPauseLabel = "budget-hard"

// Gate coordinates the smart-zone pause/resume protocol shared by every
// iteration running under a single `gx ralph-loop` invocation. Any iteration
// can pause the whole loop (stop new scheduling; block the process in
// place) without stepping on another iteration's independent pause, and
// every iteration paused at the time wakes together the moment ForceResume
// clears the last of them.
type Gate struct {
	mu         sync.Mutex
	reasons    map[string]string // iteration label -> pause reason
	wake       chan struct{}
	parkWake   chan struct{}
	draining   bool
	parked     chan struct{}
	parkedOnce sync.Once
}

func NewGate() *Gate {
	return &Gate{
		reasons:  map[string]string{},
		wake:     make(chan struct{}),
		parkWake: make(chan struct{}),
		parked:   make(chan struct{}),
	}
}

// Pause stops this Gate from admitting new claims while allowing work that
// already passed the claim boundary to finish normally.
func (g *Gate) Pause(label, reason string) {
	g.pause(label, reason)
}

// pause records label as paused for reason, leaving any other already-paused
// iteration untouched.
func (g *Gate) pause(label, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reasons[label] = reason
}

// Drain permanently stops this Gate from admitting new claims, without
// interrupting any iteration already past the claim boundary — unlike
// Pause, it never expects a ForceResume/resume-signal to arrive and clear
// it. Once every currently in-flight iteration finishes (or immediately, if
// none are in flight), Run's own active==0 check ends the run through the
// same code path natural completion takes; isDraining is what that check
// consults. It also closes wake and parkWake (mirroring ForceResume and
// WakeParked's lock/close/replace shape), so a run currently parked in
// waitForResume — nothing in flight, an operator away from the terminal —
// wakes immediately and re-evaluates isDraining instead of hanging until a
// ForceResume that Drain doesn't expect to come.
func (g *Gate) Drain() {
	g.mu.Lock()
	if g.draining {
		// Already draining: wake/parkWake were already closed and replaced
		// by the first Drain call, so closing them again would panic.
		g.mu.Unlock()
		return
	}
	g.draining = true
	wake := g.wake
	g.wake = make(chan struct{})
	parkWake := g.parkWake
	g.parkWake = make(chan struct{})
	g.mu.Unlock()

	close(wake)
	close(parkWake)
}

// isDraining reports whether Drain has been called on this Gate.
func (g *Gate) isDraining() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.draining
}

// IsDraining is isDraining, exported for callers outside this package (e.g.
// ui/tickets' drain-then-replace combo tests) that need to observe whether
// Drain landed without a package-internal accessor of their own.
func (g *Gate) IsDraining() bool {
	return g.isDraining()
}

// isPaused reports whether any iteration is currently paused, meaning the
// scheduler must not claim new tickets right now.
func (g *Gate) isPaused() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.reasons) > 0
}

// claimIfRunning serializes the pause/drain transition with the complete
// claim. Pause and Drain therefore cannot return while a claim admitted
// before them is still being recorded on disk.
func (g *Gate) claimIfRunning(claim func() error) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.draining || len(g.reasons) > 0 {
		return false, nil
	}
	return true, claim()
}

// isLabelPaused reports whether label specifically is still paused, letting
// a caller notice an in-process ForceResume (e.g. from the TUI) without
// waiting on the shared wake channel — see waitForClaudeRateLimitReset,
// which also races a reset deadline.
func (g *Gate) isLabelPaused(label string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, paused := g.reasons[label]
	return paused
}

// snapshot returns a copy of every currently-paused iteration's reason, for
// reporting.
func (g *Gate) snapshot() map[string]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]string, len(g.reasons))
	maps.Copy(out, g.reasons)
	return out
}

// waitForResume blocks the calling iteration until ForceResume clears the
// last paused label, releasing every iteration paused at that moment
// together on the shared wake channel — or until ctx is done, letting Run's
// own shutdown path unblock a run parked here without waiting on an operator.
func (g *Gate) waitForResume(ctx context.Context) {
	g.mu.Lock()
	if len(g.reasons) == 0 {
		// Nothing is paused: a ForceResume already landed between the
		// caller's pause() and this call (e.g. a TUI operator resuming the
		// instant it observes the pause).
		g.mu.Unlock()
		return
	}
	myWake := g.wake
	g.mu.Unlock()

	g.parkedOnce.Do(func() { close(g.parked) })

	select {
	case <-myWake:
	case <-ctx.Done():
	}
}

// Parked returns a channel closed the first time a caller actually blocks in
// waitForResume, letting a test observe that a run's goroutine has parked
// there instead of racing it with a fixed sleep before calling Drain.
func (g *Gate) Parked() <-chan struct{} {
	return g.parked
}

// ForceResume clears label's own pause without disturbing any other
// iteration's, in-process — reachable only by a caller holding this exact
// Gate value, i.e. one sharing this process with the running loop (the TUI).
// It reports whether label was actually paused. If no iteration is paused
// afterward, it also releases any goroutine still blocked in waitForResume.
func (g *Gate) ForceResume(label string) bool {
	g.mu.Lock()
	_, wasPaused := g.reasons[label]
	delete(g.reasons, label)
	var wake chan struct{}
	if len(g.reasons) == 0 {
		wake = g.wake
		g.wake = make(chan struct{})
	}
	g.mu.Unlock()

	if wake != nil {
		close(wake)
	}
	return wasPaused
}

// ParkWake returns the channel a parked run's poll wait selects on, closed
// by WakeParked to cut that wait short without touching the underlying
// d.Sleep(parkPollInterval) call itself.
func (g *Gate) ParkWake() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.parkWake
}

// WakeParked interrupts a parked run's current poll wait — a cosmetic wake
// that skips the remainder of the current parkPollInterval tick so a run
// whose Run() goroutine is still alive rechecks the frontier immediately,
// mirroring ForceResume's lock/close/replace shape so a second WakeParked
// call doesn't panic on an already-closed channel.
func (g *Gate) WakeParked() {
	g.mu.Lock()
	wake := g.parkWake
	g.parkWake = make(chan struct{})
	g.mu.Unlock()

	close(wake)
}
