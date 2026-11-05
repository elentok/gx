// Package ticketops holds the ticket operations a person or an agent calls
// over the socket: answer, ask, recommend and the live-agent verbs.
package ticketops

import (
	"log/slog"
	"net/http"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/epics"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
)

// Launch is what the operations need of the run loop in the root.
type Launch interface {
	// BudgetRefusal refuses new agent work past the daily soft limit.
	BudgetRefusal() (wire.QueueResult, bool)
	// ResettleWaiting ends a run's wait for an answer and settles it.
	ResettleWaiting(t runs.TrackedRun) error
	// PromptsOnRetry reports whether a retry of address prompts its live
	// agent instead of relaunching.
	PromptsOnRetry(address string) bool
	// RepairResume is the resume dispatch a prompting retry goes through.
	RepairResume(web bool) http.HandlerFunc
}

// Deps is what New needs.
type Deps struct {
	Log      *slog.Logger
	Writer   *ticketwrite.Writer
	Harness  *harnesschoice.Harness
	Registry *runs.Registry
	Index    *index.Index
	Events   *broker.Broker
	Epics    *epics.Epics
	Projects *projects.Registry
	Rec      *merge.Reconciler
	Launch   Launch
}

// Ops makes the ticket operations.
type Ops struct{ d Deps }

// New returns an Ops.
func New(d Deps) *Ops { return &Ops{d: d} }
