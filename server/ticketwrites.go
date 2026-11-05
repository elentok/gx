package server

import (
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// budgetRefusal refuses new agent work past the daily soft limit; ok is
// false when there is nothing to refuse.
func (s *Server) budgetRefusal() (wire.QueueResult, bool) {
	if !s.budget.SoftReached(time.Now()) {
		return wire.QueueResult{}, false
	}
	return queue.Refusal(wire.ReasonBudgetLimit, "the daily budget limit is reached; raise it with `gx budget override`"), true
}

// promptsOnRetry reports whether a retry of address prompts its live agent
// instead of relaunching.
func (s *Server) promptsOnRetry(address string) bool {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return false
	}
	_, t, ok := s.FreshTicket(addr)
	if !ok {
		return false
	}
	tk, err := schema.ParseTicket(t.Path)
	if err != nil || !tk.PausedAsking() {
		return false
	}
	if _, ok := retryPrompts[tk.PauseReason]; !ok {
		return false
	}
	dir, err := s.projects.Dir(addr.Project)
	if err != nil {
		return false
	}
	_, _, live, err := s.iterationSession(dir, addr)
	return err == nil && live
}
