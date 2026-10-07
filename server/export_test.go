package server

import (
	"errors"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// PutRun records a launched iteration without launching one, so tests can hold
// a run live as long as they need.
func (s *Server) PutRun(root string, r Run) { s.registry.put(trackedRun{Run: r, Root: root}) }

// ClaimTicket claims and launches one ticket the way a claim pass would, but
// without asking the frontier, and reports whether an iteration launched.
func (s *Server) ClaimTicket(address, agent string) (bool, error) {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return false, err
	}
	ref, ok, err := s.findTicket(address)
	if err != nil || !ok {
		return false, errors.Join(err, errors.New("no ticket "+address))
	}
	epics, err := tickets.Load(ref.projectDir)
	if err != nil {
		return false, err
	}
	return s.claimAndLaunch(rootOf(addr), addr, ref.ticket, ref.repo, ralphloop.AgentKind(agent), epics)
}

// SetCostOf swaps how an iteration's cost is read.
func (s *Server) SetCostOf(f func(IterationInfo) (float64, bool)) { s.costOf = f }

// PollBudget runs one budget poll now.
func (s *Server) PollBudget() { s.pollBudget(time.Now()) }

// BudgetStatusAt is the budget status as of now.
func (s *Server) BudgetStatusAt(now time.Time) BudgetStatus { return s.budgetStatus(now) }

// RecordSpend adds spend to today's ledger.
func (s *Server) RecordSpend(key string, cost float64) { s.ledger.record(key, cost, time.Now()) }
