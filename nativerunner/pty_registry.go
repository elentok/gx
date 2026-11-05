package nativerunner

import (
	"fmt"
	"sync"

	"github.com/elentok/gx/agentrunner"
)

// ptyRegistry owns Pty's session map and the labels whose Start is still
// launching. Its zero value is ready to use.
type ptyRegistry struct {
	mu       sync.Mutex
	sessions map[string]*ptySession
	// starting holds labels whose Start is still launching.
	starting map[string]struct{}
}

func (r *ptyRegistry) reserve(label string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, starting := r.starting[label]; starting {
		return fmt.Errorf("%w: %s", agentrunner.ErrLabelTaken, label)
	}
	if r.starting == nil {
		r.starting = map[string]struct{}{}
	}
	r.starting[label] = struct{}{}
	return nil
}

func (r *ptyRegistry) release(label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.starting, label)
}

func (r *ptyRegistry) live(s agentrunner.Session) (*ptySession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss, ok := r.sessions[s.Label]
	if !ok || ss.session.ID != s.ID {
		return nil, fmt.Errorf("%w: %s", agentrunner.ErrNotFound, s.Label)
	}
	if ss.died != nil {
		return nil, ss.died
	}
	return ss, nil
}

func (r *ptyRegistry) markDied(ss *ptySession, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss.died = err
}

func (r *ptyRegistry) hasDied(ss *ptySession) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return ss.died != nil
}

// stopFailed lets a later Stop retry after err and returns err.
func (r *ptyRegistry) stopFailed(ss *ptySession, err error) error {
	r.mu.Lock()
	ss.stopped = false
	r.mu.Unlock()
	return err
}

// tracked returns label's drivable in-memory session. A silent one that Find
// registered as dead doesn't count, so Reattach still re-checks it.
func (r *ptyRegistry) tracked(label string) (agentrunner.Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss, ok := r.sessions[label]
	if !ok || ss.died != nil {
		return agentrunner.Session{}, false
	}
	return ss.session, true
}

// trackSilent registers a live-pid holder that doesn't answer as a dead
// session, so every call on it fails with ErrSessionDied.
func (r *ptyRegistry) trackSilent(ss *ptySession) agentrunner.Session {
	ss.died = agentrunner.SessionDied("holder socket is silent")
	close(ss.exited)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.putLocked(ss)
	return ss.session
}

// put registers ss under its label.
func (r *ptyRegistry) put(ss *ptySession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.putLocked(ss)
}

func (r *ptyRegistry) putLocked(ss *ptySession) {
	if r.sessions == nil {
		r.sessions = map[string]*ptySession{}
	}
	r.sessions[ss.session.Label] = ss
}

// putUnlessLive registers ss under label unless a concurrent adopt already
// tracks a live one; then it returns that one's session and false. start runs
// under the lock, only when ss is registered.
func (r *ptyRegistry) putUnlessLive(label string, ss *ptySession, start func()) (agentrunner.Session, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if other, ok := r.sessions[label]; ok && other.died == nil {
		return other.session, false
	}
	start()
	r.putLocked(ss)
	return ss.session, true
}

// get returns label's session, dead or alive.
func (r *ptyRegistry) get(label string) (*ptySession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss, ok := r.sessions[label]
	return ss, ok
}

// claimStop marks s's session stopped and returns it, or nil when s is
// unknown or already stopped.
func (r *ptyRegistry) claimStop(s agentrunner.Session) *ptySession {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss, ok := r.sessions[s.Label]
	if !ok || ss.session.ID != s.ID || ss.stopped {
		return nil
	}
	ss.stopped = true
	return ss
}

func (r *ptyRegistry) remove(label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, label)
}

// labelsOf returns the labels of epic's tracked sessions.
func (r *ptyRegistry) labelsOf(epic string) map[string]struct{} {
	labels := map[string]struct{}{}
	r.mu.Lock()
	defer r.mu.Unlock()
	for label, ss := range r.sessions {
		if ss.epic == epic {
			labels[label] = struct{}{}
		}
	}
	return labels
}
