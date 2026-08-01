package ralphloop

import (
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// resumePollInterval is how often a blocked loop re-checks the resume
// signal file while paused.
const resumePollInterval = 2 * time.Second

// Gate coordinates the smart-zone pause/resume protocol shared by every
// iteration running under a single `gx ralph-loop` invocation. Any iteration
// can pause the whole loop (stop new scheduling; block the process in
// place) without stepping on another iteration's independent pause, and
// every iteration paused at the time wakes together the moment a single
// `gx ralph-loop resume` signal arrives.
type Gate struct {
	mu      sync.Mutex
	reasons map[string]string // iteration label -> pause reason
	polling bool
	wake    chan struct{}
}

func NewGate() *Gate {
	return &Gate{reasons: map[string]string{}, wake: make(chan struct{})}
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

// snapshot returns a copy of every currently-paused iteration's reason, for
// reporting.
func (g *Gate) snapshot() map[string]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]string, len(g.reasons))
	maps.Copy(out, g.reasons)
	return out
}

// waitForResume blocks the calling iteration until a resume signal appears
// at path. Only the first iteration to call this while paused actually
// polls for the signal (the "leader"); any others simply block on the
// shared wake channel and are released together once the leader observes
// the signal — so one `gx ralph-loop resume` clears every iteration paused
// at that moment, however many there are.
func (g *Gate) waitForResume(d Deps, path string) {
	g.mu.Lock()
	if len(g.reasons) == 0 {
		// Nothing is paused: a ForceResume already landed between the
		// caller's pause() and this call (e.g. a TUI operator resuming the
		// instant it observes the pause). Becoming the leader here would
		// poll forever for a resume signal that already happened.
		g.mu.Unlock()
		return
	}
	myWake := g.wake
	lead := !g.polling
	if lead {
		g.polling = true
	}
	g.mu.Unlock()

	if !lead {
		<-myWake
		return
	}

	for {
		signaled, err := d.ResumeSignaled(path)
		if err == nil && signaled {
			break
		}

		// d.Sleep runs on its own goroutine so this select can also react to
		// ForceResume immediately, instead of only rechecking ResumeSignaled
		// once the current sleep happens to elapse.
		sleepDone := make(chan struct{})
		go func() {
			d.Sleep(resumePollInterval)
			close(sleepDone)
		}()
		select {
		case <-sleepDone:
		case <-myWake:
			// ForceResume already reset/closed myWake on our behalf.
			return
		}
	}

	g.mu.Lock()
	if g.wake == myWake {
		g.reasons = map[string]string{}
		g.polling = false
		g.wake = make(chan struct{})
	}
	g.mu.Unlock()
	close(myWake)
}

// ForceResume clears label's own pause without disturbing any other
// iteration's, in-process — the same effect a `gx ralph-loop resume
// {epicName}` file signal has once every paused iteration's leader notices
// it, but immediate (no resumePollInterval wait) and reachable only by a
// caller holding this exact Gate value, i.e. one sharing this process with
// the running loop (the TUI, once a future ticket wires it up), unlike the
// file signal Resume writes for a separate, headless invocation. It reports
// whether label was actually paused. If no iteration is paused afterward, it
// also releases any goroutine still blocked in waitForResume — see that
// method's own select on this same wake channel for how it turns this into
// an instant wake rather than waiting for its next poll tick.
func (g *Gate) ForceResume(label string) bool {
	g.mu.Lock()
	_, wasPaused := g.reasons[label]
	delete(g.reasons, label)
	var wake chan struct{}
	if len(g.reasons) == 0 {
		wake = g.wake
		g.wake = make(chan struct{})
		g.polling = false
	}
	g.mu.Unlock()

	if wake != nil {
		close(wake)
	}
	return wasPaused
}

// resumeSignalPath is where Resume writes its wake signal for a blocked
// `gx ralph-loop {epicName}` invocation, and where that invocation polls
// for it.
func resumeSignalPath(scratchDir, epicName string) string {
	return filepath.Join(scratchDir, epicName, "resume-signal")
}

// Resume signals a `gx ralph-loop {epicName}` invocation blocked on a
// smart-zone pause to wake up and resume scheduling. If no invocation is
// currently paused, the signal is simply left in place and consumed
// harmlessly the next time one pauses and starts polling.
func Resume(scratchDir, epicName string) error {
	if scratchDir == "" {
		scratchDir = defaultScratchDir
	}
	path := resumeSignalPath(scratchDir, epicName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, nil, 0644)
}

// defaultResumeSignaled implements Deps.ResumeSignaled against the real
// filesystem: the signal is "consumed" (removed) the moment it's observed,
// so a stale signal from a previous pause never fires early on the next
// one.
func defaultResumeSignaled(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}
