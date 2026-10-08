package server

import (
	"sync"
	"time"
)

// recoveryHold keeps a park's chat message back while recovery works on the
// ticket. Exactly one of three things then happens to it: recovery succeeds
// and drops it, recovery fails and sends it, or the cap expires and sends it.
type recoveryHold struct {
	mu   sync.Mutex
	held map[string]*time.Timer // ticket address -> cap timer
}

// hold arms the cap timer; send runs if the cap expires before take.
func (h *recoveryHold) hold(key string, cap time.Duration, send func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.held == nil {
		h.held = map[string]*time.Timer{}
	}
	if old := h.held[key]; old != nil {
		old.Stop()
	}
	var t *time.Timer
	t = time.AfterFunc(cap, func() {
		// Locking first orders the read of t after hold has assigned it.
		h.mu.Lock()
		mine := h.held[key] == t
		if mine {
			delete(h.held, key)
		}
		h.mu.Unlock()
		if mine {
			send()
		}
	})
	h.held[key] = t
}

// take removes the held message and reports whether one was still held.
func (h *recoveryHold) take(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.held[key]
	if ok {
		t.Stop()
		delete(h.held, key)
	}
	return ok
}
