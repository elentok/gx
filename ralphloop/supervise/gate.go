package supervise

import (
	"context"
	"maps"
	"sync"
)

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
	pauseWake  chan struct{}
	paused     chan struct{}
	pausedOnce sync.Once
}

func NewGate() *Gate {
	return &Gate{
		reasons:   map[string]string{},
		wake:      make(chan struct{}),
		pauseWake: make(chan struct{}),
		paused:    make(chan struct{}),
	}
}

// Pause stops this Gate from admitting new claims while allowing work that
// already passed the claim boundary to finish normally. It records label as
// paused for reason, leaving any other already-paused iteration untouched.
func (g *Gate) Pause(label, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reasons[label] = reason
}

// IsPaused reports whether any iteration is currently paused, meaning the
// scheduler must not claim new tickets right now.
func (g *Gate) IsPaused() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.reasons) > 0
}

// ClaimIfRunning serializes the pause transition with the complete claim.
// Pause therefore cannot return while a claim admitted before it is still
// being recorded on disk.
func (g *Gate) ClaimIfRunning(claim func() error) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.reasons) > 0 {
		return false, nil
	}
	return true, claim()
}

// Snapshot returns a copy of every currently-paused iteration's reason, for
// reporting.
func (g *Gate) Snapshot() map[string]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]string, len(g.reasons))
	maps.Copy(out, g.reasons)
	return out
}

// WaitForResume blocks the calling iteration until ForceResume clears the
// last paused label, releasing every iteration paused at that moment
// together on the shared wake channel — or until ctx is done, letting Run's
// own shutdown path unblock a run paused here without waiting on an operator.
func (g *Gate) WaitForResume(ctx context.Context) {
	g.mu.Lock()
	if len(g.reasons) == 0 {
		// Nothing is paused: a ForceResume already merged between the
		// caller's Pause() and this call (e.g. a TUI operator resuming the
		// instant it observes the pause).
		g.mu.Unlock()
		return
	}
	myWake := g.wake
	g.mu.Unlock()

	g.pausedOnce.Do(func() { close(g.paused) })

	select {
	case <-myWake:
	case <-ctx.Done():
	}
}

// Paused returns a channel closed the first time a caller actually blocks in
// WaitForResume, letting a test observe that a run's goroutine has paused
// there instead of racing it with a fixed sleep.
func (g *Gate) Paused() <-chan struct{} {
	return g.paused
}

// ForceResume clears label's own pause without disturbing any other
// iteration's, in-process — reachable only by a caller holding this exact
// Gate value, i.e. one sharing this process with the running loop (the TUI).
// It reports whether label was actually paused. If no iteration is paused
// afterward, it also releases any goroutine still blocked in WaitForResume.
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

// PauseWake returns the channel a paused run's poll wait selects on, closed
// by WakePaused to cut that wait short without touching the underlying
// d.Sleep call itself.
func (g *Gate) PauseWake() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.pauseWake
}

// WakePaused interrupts a paused run's current poll wait — a cosmetic wake
// that skips the remainder of the current poll tick so a run
// whose Run() goroutine is still alive rechecks the frontier immediately,
// mirroring ForceResume's lock/close/replace shape so a second WakePaused
// call doesn't panic on an already-closed channel.
func (g *Gate) WakePaused() {
	g.mu.Lock()
	wake := g.pauseWake
	g.pauseWake = make(chan struct{})
	g.mu.Unlock()

	close(wake)
}
