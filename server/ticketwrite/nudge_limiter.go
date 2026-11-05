package ticketwrite

import (
	"sync"
	"time"
)

const (
	NudgeLimit  = 3
	NudgeWindow = time.Minute
)

// NudgeLimiter allows NudgeLimit nudges per iteration in any NudgeWindow, so a
// looping caller cannot bury a pane in text. The zero value is ready to use.
type NudgeLimiter struct {
	mu   sync.Mutex
	sent map[string][]time.Time
}

// Allow records a nudge for address and reports whether it is within the limit.
func (l *NudgeLimiter) Allow(address string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sent == nil {
		l.sent = map[string][]time.Time{}
	}
	recent := l.sent[address][:0]
	for _, t := range l.sent[address] {
		if now.Sub(t) < NudgeWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= NudgeLimit {
		l.sent[address] = recent
		return false
	}
	l.sent[address] = append(recent, now)
	return true
}
