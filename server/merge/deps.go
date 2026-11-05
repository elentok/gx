package merge

import (
	"log/slog"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server/broker"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Host is what Merger, Landing and Reconciler need from the root that is not a
// plain dependency.
type Host interface {
	RootRefs(project string, epics []tickets.Epic, epic string, t tickets.Ticket, repo string) (base, target string, err error)
	HostingDeps(addr tickets.Address) ralphloop.Deps
	MergeTarget(addr tickets.Address) (string, error)
	FreshTicket(addr tickets.Address) (tickets.Epic, tickets.Ticket, bool)
	ResolveRun(t runs.TrackedRun) (ralphloop.OneIteration, ralphloop.IterationWorktree, IterationMode, error)
	LogEvent(ticket tickets.Address, ev runlog.Event)
	RecoverAsync(f recovery.Failure)
	// EpicState and EpicMergeBlocked answer for the epic list's rules, which
	// stay in the root until the epic summary moves.
	EpicState(root queue.RootRef, e tickets.Epic) wire.EpicState
	EpicMergeBlocked(e tickets.Epic) bool
}

// Dirs is the part of the server's configuration the merge code reads.
type Dirs struct {
	StateDir    string
	TicketStore string
}

// Deps is everything the merge types copy when they are built. A field the
// root swaps later is not seen.
type Deps struct {
	Host     Host
	Log      *slog.Logger
	Dirs     Dirs
	Idx      *index.Index
	Projects *projects.Registry
	Queue    *queue.Queue
	OneOffs  *queue.OneOffs
	Events   *broker.Broker
	Writer   *ticketwrite.Writer
	Chat     *ralphloop.ServerChat
	Registry *runs.Registry
	Guards   *runs.Guards
	Settings *settings.Holder
	Harness  *harnesschoice.Harness
	Sessions *sessions.Manager
}

// New builds the wired Reconciler, Landing and Merger.
func New(d Deps) (*Reconciler, *Landing, *Merger) {
	rec := NewReconciler(d)
	landing := NewLanding(d)
	merger := NewMerger(d, rec, landing)
	rec.merger = merger
	landing.merger = merger
	return rec, landing, merger
}
