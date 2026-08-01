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

// pauseGate coordinates the smart-zone pause/resume protocol shared by every
// iteration running under a single `gx ralph-loop` invocation. Any iteration
// can pause the whole loop (stop new scheduling; block the process in
// place) without stepping on another iteration's independent pause, and
// every iteration paused at the time wakes together the moment a single
// `gx ralph-loop resume` signal arrives.
type pauseGate struct {
	mu      sync.Mutex
	reasons map[string]string // iteration label -> pause reason
	polling bool
	wake    chan struct{}
}

func newPauseGate() *pauseGate {
	return &pauseGate{reasons: map[string]string{}, wake: make(chan struct{})}
}

// pause records label as paused for reason, leaving any other already-paused
// iteration untouched.
func (g *pauseGate) pause(label, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reasons[label] = reason
}

// isPaused reports whether any iteration is currently paused, meaning the
// scheduler must not claim new tickets right now.
func (g *pauseGate) isPaused() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.reasons) > 0
}

// snapshot returns a copy of every currently-paused iteration's reason, for
// reporting.
func (g *pauseGate) snapshot() map[string]string {
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
func (g *pauseGate) waitForResume(d Deps, path string) {
	g.mu.Lock()
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
		d.Sleep(resumePollInterval)
	}

	g.mu.Lock()
	g.reasons = map[string]string{}
	g.polling = false
	g.wake = make(chan struct{})
	g.mu.Unlock()
	close(myWake)
}

// resumeLabel clears label's own pause without disturbing any other
// iteration's, for a pause that resumes on its own schedule (e.g. a
// rate-limit reset) rather than via a shared external resume signal. If no
// iteration is paused afterward, it also releases any goroutine still
// blocked in waitForResume, matching that method's own group-wake-on-clear
// behavior.
func (g *pauseGate) resumeLabel(label string) {
	g.mu.Lock()
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
