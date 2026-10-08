package server

import (
	"sync"
	"time"
)

// TicketInfo.Recovery values.
const (
	RecoveryPending   = "pending"
	RecoveryEscalated = "escalated"
)

// recoveryHold keeps a park's chat message back while recovery works on the
// ticket. Exactly one of three things then happens to it: recovery succeeds
// and drops it, recovery fails and sends it, or the cap expires and sends it.
type recoveryHold struct {
	mu   sync.Mutex
	held map[string]*time.Timer // ticket address -> cap timer
	// escalated are the tickets whose held park ended in a failed remedy, until
	// their next park.
	escalated map[string]bool
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

// escalate marks the ticket's held park as given up on by recovery.
func (h *recoveryHold) escalate(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.escalated == nil {
		h.escalated = map[string]bool{}
	}
	h.escalated[key] = true
}

// forget clears an escalation, for a new park of the ticket.
func (h *recoveryHold) forget(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.escalated, key)
}

// state is the ticket's TicketInfo.Recovery.
func (h *recoveryHold) state(key string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.held[key] != nil:
		return RecoveryPending
	case h.escalated[key]:
		return RecoveryEscalated
	}
	return ""
}
