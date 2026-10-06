package server

import (
	"fmt"
	"sync"
	"time"
)

// parkFoldCap bounds how long a herdr-caused park waits for herdr to return.
const parkFoldCap = 10 * time.Minute

// parkFold holds herdr-caused parks during an outage so a long one yields one
// digest instead of a message per park.
type parkFold struct {
	mu    sync.Mutex
	lines []string
	timer *time.Timer
}

// hold queues a park line and arms the cap timer on the first one; flush runs
// when the cap expires.
func (f *parkFold) hold(line string, flush func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, line)
	if f.timer == nil {
		f.timer = time.AfterFunc(parkFoldCap, flush)
	}
}

// drain returns the held lines and resets the fold.
func (f *parkFold) drain() []string {
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

func (s *Server) flushParkFold() {
	s.chat.ParkDigest(s.parkFold.drain())
}

// holdPark queues the park line for the digest.
func (s *Server) holdPark(project, epic, id, status string) {
	s.parkFold.hold(fmt.Sprintf("[%s] %s/%s — %s", project, epic, id, status), s.flushParkFold)
}
