package server

import (
	"sync"
	"time"

	"github.com/elentok/gx/tickets/schema"
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
	// starting are the parks whose unheld recovery has not yet escalated or
	// opened its investigation, so a --wait cannot slip out in between.
	starting map[string]bool
}

// start marks the park's unheld recovery as under way.
func (h *recoveryHold) start(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.starting == nil {
		h.starting = map[string]bool{}
	}
	h.starting[key] = true
}

// started clears start and reports whether it was still set.
func (h *recoveryHold) started(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	ok := h.starting[key]
	delete(h.starting, key)
	return ok
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
	delete(h.starting, key)
}

// forget clears an escalation, for a new park of the ticket.
func (h *recoveryHold) forget(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.escalated, key)
}

// recoveryState is a parked ticket's TicketInfo.Recovery: its hold's state,
// else what its investigations say. It is read from the index rather than
// kept, so a server restart mid-investigation still reports it. An open
// investigation is pending; a parked one is a person's, so recovery escalated.
func recoveryState(held, address string, all []TicketInfo) string {
	if held != "" {
		return held
	}
	state := ""
	for _, t := range all {
		if t.Parent != address || t.Type != string(schema.TypeInvestigate) {
			continue
		}
		switch schema.Status(t.Status) {
		case schema.StatusDone, schema.StatusCancelled:
		case schema.StatusNeedsAnswer, schema.StatusNeedsRepair:
			state = RecoveryEscalated
		default:
			return RecoveryPending
		}
	}
	return state
}

// state is the hold's part of the ticket's TicketInfo.Recovery.
func (h *recoveryHold) state(key string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.held[key] != nil, h.starting[key]:
		return RecoveryPending
	case h.escalated[key]:
		return RecoveryEscalated
	}
	return ""
}
