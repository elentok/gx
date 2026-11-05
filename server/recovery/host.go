// Package recovery is the server's failure recovery: it plans what a failed
// ticket gets (a rule remedy, an investigation, a proposal or an escalation),
// runs it through the server's own verbs, and files the follow-ups an
// investigation proposes.
package recovery

import (
	"log/slog"
	"sync"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/broker"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
)

// Host is what recovery needs of the root.
type Host interface {
	// LiveIteration resolves a ticket's live iteration.
	LiveIteration(address string) (ticketwrite.LiveIter, bool)
}

// Deps is what New needs.
type Deps struct {
	Log      *slog.Logger
	Settings *settings.Holder
	Projects *projects.Registry
	Migrator *projects.Migrator
	Index    *index.Index
	Queue    *queue.Queue
	Events   *broker.Broker
	Guards   *runs.Guards
	Registry *runs.Registry
	Harness  *harnesschoice.Harness
	Chat     *ralphloop.ServerChat
	Writer   *ticketwrite.Writer
	// Hold is the pause hold the server also reads for a ticket's recovery state.
	Hold *ticketwrite.RecoveryHold
	// CompactRewaitTimeout bounds R7's extended wait; zero means
	// DefaultCompactRewaitTimeout.
	CompactRewaitTimeout time.Duration
	Host                 Host
}

// Recovery plans and runs failure recovery.
type Recovery struct {
	d Deps
	// filedRounds holds the round markers already in the follow-ups epic, so an
	// Idle detection never rescans its tickets.
	filedRounds sync.Map
}

// New returns a Recovery.
func New(d Deps) *Recovery { return &Recovery{d: d} }

// Routes lists the recovery endpoints. The getter is called per request, so a
// route can be listed before its Recovery is built.
func Routes(get func() *Recovery) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/tickets/approve", Handler: queue.Handler(func(req wire.QueueRequest) (wire.QueueResult, error) { return get().ticketApprove(req) })},
	}
}
