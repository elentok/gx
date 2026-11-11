package server

import (
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"time"
)

// budgetRefusal refuses new agent work past the daily soft limit; ok is
// false when there is nothing to refuse.
func budgetRefusal(s *Server) (wire.QueueResult, bool) {
	if !s.budget.SoftReached(time.Now()) {
		return wire.QueueResult{}, false
	}
	return queue.Refusal(wire.ReasonBudgetLimit, "the daily budget limit is reached; raise it with `gx budget override`"), true
}

// PauseAs pauses a ticket as the given kind the way the server's own pauses do,
// including starting recovery. Tests in other packages call it, so it cannot
// live in an export_test.go.
func (s *Server) PauseAs(address string, kind events.Kind, reason string) error {
	return s.writer.PauseAs(address, kind, reason)
}
