package server

import (
	"log/slog"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
)

// landingHost is what Landing needs from the root Server that is not a plain
// dependency.
type landingHost interface {
	mergeTarget(addr tickets.Address) (string, error)
	FreshTicket(addr tickets.Address) (tickets.Epic, tickets.Ticket, bool)
}

// Landing is the server's dirty-trunk landing and trunk-conflict code: the
// backup-and-restore land, the stash merge, the trunk-conflict finish and
// start-over, and the iteration mode choice. Its dependencies are fields
// copied from Server when it is built; the rest of the root comes in through
// host. merger is set by New once the Merger exists.
type Landing struct {
	host     landingHost
	log      *slog.Logger
	cfg      Config
	idx      *index.Index
	projects *projects.Registry
	events   *broker.Broker
	writer   *ticketwrite.Writer
	guards   *runs.Guards
	merger   *Merger
}

func newLanding(s *Server) *Landing {
	return &Landing{
		host: s, log: s.log, cfg: s.cfg, idx: s.idx, projects: s.projects,
		events: s.events, writer: s.writer, guards: s.guards,
	}
}
