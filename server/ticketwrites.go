package server

import (
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
)

// budgetRefusal refuses new agent work past the daily soft limit; ok is
// false when there is nothing to refuse.
func budgetRefusal(s *Server) (wire.QueueResult, bool) {
	if !s.budget.SoftReached(time.Now()) {
		return wire.QueueResult{}, false
	}
	return queue.Refusal(wire.ReasonBudgetLimit, "the daily budget limit is reached; raise it with `gx budget override`"), true
}
