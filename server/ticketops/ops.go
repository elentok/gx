// Package ticketops holds the ticket operations a person or an agent calls
// over the socket: answer, ask, recommend and the live-agent verbs.
package ticketops

import (
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/epics"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
)

// Launch is what the operations need of the run loop in the root.
type Launch interface {
	// BudgetRefusal refuses new agent work past the daily soft limit.
	BudgetRefusal() (wire.QueueResult, bool)
	// CancelInvestigateChild withdraws ref's unfinished investigate child.
	CancelInvestigateChild(ref queue.TicketRef, source string) error
	// ResettleWaiting ends a run's wait for an answer and settles it.
	ResettleWaiting(t runs.TrackedRun) error
}

// Deps is what New needs.
type Deps struct {
	Writer   *ticketwrite.Writer
	Harness  *harnesschoice.Harness
	Registry *runs.Registry
	Index    *index.Index
	Events   *broker.Broker
	Epics    *epics.Epics
	Rec      *merge.Reconciler
	Launch   Launch
}

// Ops makes the ticket operations.
type Ops struct{ d Deps }

// New returns an Ops.
func New(d Deps) *Ops { return &Ops{d: d} }
