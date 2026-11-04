package budget

import (
	"time"

	"github.com/elentok/gx/server/wire"
)

// The methods below are for tests, which reach them through Server.Budget().

// SetCostOf swaps how an iteration's cost is read. For tests.
func (b *Budget) SetCostOf(f func(wire.IterationInfo) (float64, bool)) { b.costOf = f }

// HardGraceRunning reports whether a hard-limit countdown is running. For tests.
func (b *Budget) HardGraceRunning() bool { return !b.grace.deadline().IsZero() }

// RecordSpend adds spend to today's ledger. For tests.
func (b *Budget) RecordSpend(key string, cost float64) { b.ledger.record("", key, cost, time.Now()) }

// RecordProjectSpend adds spend attributed to a project to today's ledger. For tests.
func (b *Budget) RecordProjectSpend(project, key string, cost float64) {
	b.ledger.record(project, key, cost, time.Now())
}

// RecordSpendAt adds spend to the ledger as of now. For tests.
func (b *Budget) RecordSpendAt(key string, cost float64, now time.Time) {
	b.ledger.record("", key, cost, now)
}

// NotifyAt runs the budget notices as of now. For tests.
func (b *Budget) NotifyAt(now time.Time) { b.notify(now) }

// OverrideAt is the override as of now. For tests.
func (b *Budget) OverrideAt(now time.Time) (wire.BudgetResult, error) { return b.override(now) }

// IncreaseAt is the increase as of now. For tests.
func (b *Budget) IncreaseAt(now time.Time, req wire.BudgetIncreaseRequest) (wire.BudgetResult, error) {
	return b.increase(now, req)
}

// PollBudget runs one budget poll now. For tests.
func (b *Budget) PollBudget() { b.poll(time.Now()) }

// LaunchingRuns counts runs whose agent has no session yet, which a budget
// kill leaves alone. For tests.
func (b *Budget) LaunchingRuns() int {
	return len(b.registry.List()) - len(b.KillableRuns())
}
