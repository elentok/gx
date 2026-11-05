package merge

import (
	"log/slog"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketwrite"
)

// Landing is the server's dirty-trunk landing and trunk-conflict code: the
// backup-and-restore land, the stash merge, the trunk-conflict finish and
// start-over, and the iteration mode choice. Its dependencies are fields
// copied from Deps when it is built; the rest of the root comes in through
// host. merger is set by New once the Merger exists.
type Landing struct {
	host     Host
	log      *slog.Logger
	cfg      Dirs
	idx      *index.Index
	projects *projects.Registry
	events   *broker.Broker
	writer   *ticketwrite.Writer
	guards   *runs.Guards
	merger   *Merger
}

// NewLanding builds a Landing from d; New sets its merger.
func NewLanding(d Deps) *Landing {
	return &Landing{
		host: d.Host, log: d.Log, cfg: d.Dirs, idx: d.Idx, projects: d.Projects,
		events: d.Events, writer: d.Writer, guards: d.Guards,
	}
}
