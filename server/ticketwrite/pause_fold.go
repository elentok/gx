package ticketwrite

import (
	"sync"
	"time"
)

// pauseFoldCap bounds how long a herdr-caused pause waits for herdr to return.
const pauseFoldCap = 10 * time.Minute

// PauseFold holds herdr-caused pauses during an outage so a long one yields one
// digest instead of a message per pause.
type PauseFold struct {
	mu    sync.Mutex
	lines []string
	timer *time.Timer
	// OnCap runs when the cap expires; set once in New, before any Hold.
	OnCap func()
}

// Hold queues a pause line and arms the cap timer on the first one.
func (f *PauseFold) Hold(line string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, line)
	if f.timer == nil {
		f.timer = time.AfterFunc(pauseFoldCap, f.OnCap)
	}
}

// Flush returns the held lines and resets the fold.
func (f *PauseFold) Flush() []string {
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
