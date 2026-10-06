package server

import "time"

// PutRun records a launched iteration without launching one, so tests can hold
// a run live as long as they need.
func (s *Server) PutRun(root string, r Run) { s.registry.put(trackedRun{Run: r, Root: root}) }

// SetCostOf swaps how an iteration's cost is read.
func (s *Server) SetCostOf(f func(IterationInfo) (float64, bool)) { s.costOf = f }

// PollBudget runs one budget poll now.
func (s *Server) PollBudget() { s.pollBudget(time.Now()) }

// BudgetStatusAt is the budget status as of now.
func (s *Server) BudgetStatusAt(now time.Time) BudgetStatus { return s.budgetStatus(now) }

// RecordSpend adds spend to today's ledger.
func (s *Server) RecordSpend(key string, cost float64) { s.ledger.record(key, cost, time.Now()) }
