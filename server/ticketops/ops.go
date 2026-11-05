// Package ticketops holds the ticket operations a person or an agent calls
// over the socket: answer, ask, recommend and the live-agent verbs.
package ticketops

import (
	"log/slog"

	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/epics"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Launch is what the operations need of the run loop in the root.
type Launch interface {
	// BudgetRefusal refuses new agent work past the daily soft limit.
	BudgetRefusal() (wire.QueueResult, bool)
	// ResettleWaiting ends a run's wait for an answer and settles it.
	ResettleWaiting(t runs.TrackedRun) error
	// HostingDeps talks to the runner hosting addr's iteration, so start-over
	// and merge see (and close) the agent where it actually runs.
	HostingDeps(addr tickets.Address) deps.Deps
	// CompleteMergedRoot completes addr's queued root as if its iteration had
	// settled, once a merge of that ticket landed. A root not queued is left alone.
	CompleteMergedRoot(addr tickets.Address)
}

// Deps is what New needs.
type Deps struct {
	Writer      *ticketwrite.Writer
	Harness     *harnesschoice.Harness
	Registry    *runs.Registry
	Index       *index.Index
	Events      *broker.Broker
	Epics       *epics.Epics
	Rec         *merge.Reconciler
	Landing     *merge.Landing
	Projects    *projects.Registry
	Log         *slog.Logger
	TicketStore string
	Launch      Launch
}

// Ops makes the ticket operations.
type Ops struct{ d Deps }

// New returns an Ops.
func New(d Deps) *Ops { return &Ops{d: d} }
