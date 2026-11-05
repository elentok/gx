package ticketwrite

import (
	"sync"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

// RecoveryHold keeps a pause's chat message back while recovery works on the
// ticket. Exactly one of three things then happens to it: recovery succeeds
// and drops it, recovery fails and sends it, or the cap expires and sends it.
type RecoveryHold struct {
	mu   sync.Mutex
	held map[string]*time.Timer // ticket address -> cap timer
	// escalated are the tickets whose held pause ended in a failed remedy, until
	// their next pause.
	escalated map[string]bool
	// starting are the pauses whose unheld recovery has not yet escalated or
	// opened its investigation, so a --wait cannot slip out in between.
	starting map[string]bool
}

// Start marks the pause's unheld recovery as under way.
func (h *RecoveryHold) Start(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.starting == nil {
		h.starting = map[string]bool{}
	}
	h.starting[key] = true
}

// Started clears Start and reports whether it was still set.
func (h *RecoveryHold) Started(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	ok := h.starting[key]
	delete(h.starting, key)
	return ok
}

// Hold arms the cap timer; send runs if the cap expires before Take.
func (h *RecoveryHold) Hold(key string, cap time.Duration, send func()) {
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
		// Locking first orders the read of t after Hold has assigned it.
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

// Take removes the held message and reports whether one was still held.
func (h *RecoveryHold) Take(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.held[key]
	if ok {
		t.Stop()
		delete(h.held, key)
	}
	return ok
}

// Escalate marks the ticket's held pause as given up on by recovery.
func (h *RecoveryHold) Escalate(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.escalated == nil {
		h.escalated = map[string]bool{}
	}
	h.escalated[key] = true
	delete(h.starting, key)
}

// Forget clears an escalation, for a new pause of the ticket.
func (h *RecoveryHold) Forget(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.escalated, key)
}

// RecoveryState is a paused ticket's TicketInfo.Recovery: its hold's state,
// else what its investigations say. It is read from the index rather than
// kept, so a server restart mid-investigation still reports it. An open
// investigation is pending; a paused one is a person's, so recovery escalated.
func RecoveryState(held, address string, all []wire.TicketInfo) string {
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
		case schema.StatusPaused, schema.StatusLegacyAnswer, schema.StatusLegacyRepair:
			state = wire.RecoveryEscalated
		default:
			return wire.RecoveryPending
		}
	}
	return state
}

// State is the hold's part of the ticket's TicketInfo.Recovery.
func (h *RecoveryHold) State(key string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.held[key] != nil, h.starting[key]:
		return wire.RecoveryPending
	case h.escalated[key]:
		return wire.RecoveryEscalated
	}
	return ""
}
