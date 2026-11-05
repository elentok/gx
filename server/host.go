package server

import (
	"log/slog"

	"github.com/elentok/gx/ralphloop"
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
	rootRefs(project string, epics []tickets.Epic, epic string, t tickets.Ticket, repo string) (base, target string, err error)
	hostingDeps(addr tickets.Address) ralphloop.Deps
	mergeTarget(addr tickets.Address) (string, error)
	FreshTicket(addr tickets.Address) (tickets.Epic, tickets.Ticket, bool)
	resolveRun(t runs.TrackedRun) (ralphloop.OneIteration, ralphloop.IterationWorktree, IterationMode, error)
	logEvent(ticket tickets.Address, ev ralphloop.Event)
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

// mergeDeps reads the fields the merge types copy; call it once they are set.
func (s *Server) mergeDeps() Deps {
	return Deps{
		Host: s, Log: s.log, Dirs: Dirs{StateDir: s.cfg.StateDir, TicketStore: s.cfg.TicketStore},
		Idx: s.idx, Projects: s.projects, Queue: s.queue, OneOffs: s.oneOffs, Events: s.events,
		Writer: s.writer, Chat: s.chat, Registry: s.registry, Guards: s.guards, Settings: &s.settings,
		Harness: s.harness, Sessions: s.sessions,
	}
}

func (s *Server) RecoverAsync(f recovery.Failure) { s.recovery.RecoverAsync(f) }

func (s *Server) EpicState(root queue.RootRef, e tickets.Epic) wire.EpicState {
	return epicStateFor(root, e, s.registry.StartedAtOf)
}

func (s *Server) EpicMergeBlocked(e tickets.Epic) bool { return epicMergeBlocked(e) }
