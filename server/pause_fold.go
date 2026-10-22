package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

// pauseFoldCap bounds how long a herdr-caused pause waits for herdr to return.
const pauseFoldCap = 10 * time.Minute

// pauseFold holds herdr-caused pauses during an outage so a long one yields one
// digest instead of a message per pause.
type pauseFold struct {
	mu    sync.Mutex
	lines []string
	timer *time.Timer
}

// hold queues a pause line and arms the cap timer on the first one; flush runs
// when the cap expires.
func (f *pauseFold) hold(line string, flush func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, line)
	if f.timer == nil {
		f.timer = time.AfterFunc(pauseFoldCap, flush)
	}
}

// flush returns the held lines and resets the fold.
func (f *pauseFold) flush() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
	lines := f.lines
	f.lines = nil
	return lines
}

func (s *Server) flushPauseFold() {
	s.chat.PauseDigest(s.pauseFold.flush())
}

// holdPause queues the pause line for the digest.
func (s *Server) holdPause(addr tickets.Address, status string, kind events.Kind, reason string) {
	s.pauseFold.hold(fmt.Sprintf("[%s] %s/%s — %s (%s): %s", addr.Project, addr.Epic, addr.ID, status, kind, reason), s.flushPauseFold)
}
