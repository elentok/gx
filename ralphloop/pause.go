package ralphloop

import (
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

// Gate coordinates the smart-zone pause/resume protocol shared by every
// iteration running under a single `gx ralph-loop` invocation. Any iteration
// can pause the whole loop (stop new scheduling; block the process in
// place) without stepping on another iteration's independent pause, and
// every iteration paused at the time wakes together the moment ForceResume
// clears the last of them.
type Gate struct {
	mu      sync.Mutex
	reasons map[string]string // iteration label -> pause reason
	wake    chan struct{}
}

func NewGate() *Gate {
	return &Gate{reasons: map[string]string{}, wake: make(chan struct{})}
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

// isPaused reports whether any iteration is currently paused, meaning the
// scheduler must not claim new tickets right now.
func (g *Gate) isPaused() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.reasons) > 0
}

// claimIfRunning serializes the pause transition with the complete claim.
// Pause therefore cannot return while a claim admitted before it is still
// being recorded on disk.
func (g *Gate) claimIfRunning(claim func() error) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.reasons) > 0 {
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
// together on the shared wake channel.
func (g *Gate) waitForResume() {
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

	<-myWake
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
