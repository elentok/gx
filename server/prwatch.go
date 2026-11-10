package server

import (
	"path/filepath"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/epics/prstate"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// PRWatcher is the watcher of open PRs, for tests that force a pass with Tick.
func (s *Server) PRWatcher() *prflow.Watcher { return s.prWatch }

// RescanIndex re-reads the ticket store now, for tests that write a ticket and
// then force a pass that reads the index.
func (s *Server) RescanIndex() error { return s.idx.Refresh() }

// prWatchHost is the root as the PR watcher sees it.
type prWatchHost struct{ s *Server }

// Watched is every epic with a stamped PR and no merged_at, read from the index.
func (h prWatchHost) Watched() []prflow.Target {
	dirs, err := tickets.ProjectDirs(h.s.cfg.TicketStore)
	if err != nil {
		return nil
	}
	var out []prflow.Target
	for _, dir := range dirs {
		epics, ok := h.s.idx.EpicsOf(dir)
		if !ok {
			continue
		}
		for _, e := range epics {
			if e.PRNumber == 0 || e.PRURL == "" || !e.MergedAt.IsZero() {
				continue
			}
			out = append(out, prflow.Target{Project: tickets.ProjectName(dir), Epic: e.Name, Dir: e.Path, Number: e.PRNumber, URL: e.PRURL})
		}
	}
	return out
}

// AutoPR is each Awaiting PR epic of a project whose delivery is `pr` and whose
// auto-pr is on. project.json is read on every call, so a flip applies on the
// next tick. A merge project never qualifies.
func (h prWatchHost) AutoPR() []prflow.Target {
	dirs, err := tickets.ProjectDirs(h.s.cfg.TicketStore)
	if err != nil {
		return nil
	}
	var out []prflow.Target
	for _, dir := range dirs {
		pf, err := config.ReadProjectFile(dir)
		if err != nil || !pf.DeliveryIsPR() || !pf.AutoPREnabled() {
			continue
		}
		epics, ok := h.s.idx.EpicsOf(dir)
		if !ok {
			continue
		}
		for _, e := range epics {
			if tickets.IsOneOffsEpicName(e.Name) || prstate.Of(e, config.DeliveryPR) != wire.EpicStateAwaitingPR {
				continue
			}
			out = append(out, prflow.Target{Project: tickets.ProjectName(dir), Epic: e.Name, Dir: e.Path})
		}
	}
	return out
}

func (h prWatchHost) OpenPR(root queue.RootRef) (prflow.Outcome, error) { return h.s.OpenPR(root) }

func (h prWatchHost) Env() []string { return h.s.cfg.Env }

func (h prWatchHost) NoChecksGrace() time.Duration { return h.s.settings.Load().NoChecksGrace }

func (h prWatchHost) RepoDir(project string) (string, bool) {
	pf, err := config.ReadProjectFile(filepath.Join(h.s.cfg.TicketStore, project))
	if err != nil || pf.Repo == nil {
		return "", false
	}
	return *pf.Repo, true
}
