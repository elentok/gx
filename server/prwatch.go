package server

import (
	"cmp"
	"fmt"
	"path/filepath"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/epics/prstate"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/recovery"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
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

// CancelStalePRText cancels, with its agent, the pr-text ticket of an epic with
// no PR whose project switched from `pr` to `merge`, so no PR opens. An epic
// with a PR is left alone: its PR stays tracked.
func (h prWatchHost) CancelStalePRText() {
	dirs, err := tickets.ProjectDirs(h.s.cfg.TicketStore)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		if pf, err := config.ReadProjectFile(dir); err != nil || pf.DeliveryIsPR() {
			continue
		}
		epics, ok := h.s.idx.EpicsOf(dir)
		if !ok {
			continue
		}
		for _, e := range epics {
			if e.PRNumber != 0 || !e.MergedAt.IsZero() {
				continue
			}
			for _, t := range e.Tickets {
				if t.Type != string(schema.TypePRText) || (t.Status != string(schema.StatusOpen) && t.Status != string(schema.StatusClaimed)) {
					continue
				}
				addr := tickets.Address{Project: tickets.ProjectName(dir), Epic: e.Name, ID: t.Identifier}
				_, _ = h.s.ops.Cancel(wire.QueueRequest{Address: addr.String(), Stop: true, Source: "pr-flow"})
			}
		}
	}
}

func (h prWatchHost) OpenPR(root queue.RootRef) (prflow.Outcome, error) { return h.s.OpenPR(root) }

// PauseCIFailing stores "CI failing" on the epic and sends its one notice.
func (h prWatchHost) PauseCIFailing(t prflow.Target, reason string) error {
	e, ok := prOpenHost(h).Epic(t.Project, t.Epic)
	if !ok {
		return fmt.Errorf("epic %s:%s not found", t.Project, t.Epic)
	}
	h.s.merger.PauseEpic(queue.RootRef{Project: t.Project, Epic: t.Epic}, e, events.CIFailing, reason)
	return nil
}

// PauseForeignConflict stores the foreign-commits block on the epic and sends its one notice.
func (h prWatchHost) PauseForeignConflict(t prflow.Target, reason string) error {
	e, ok := prOpenHost(h).Epic(t.Project, t.Epic)
	if !ok {
		return fmt.Errorf("epic %s:%s not found", t.Project, t.Epic)
	}
	h.s.merger.PauseEpic(queue.RootRef{Project: t.Project, Epic: t.Epic}, e, events.PRForeignConflict, reason)
	return nil
}

func (h prWatchHost) ForkPRMerge(root queue.RootRef) error { return h.s.merger.ForkPRMerge(root) }

func (h prWatchHost) CIFixSkills(project string) []string { return h.s.projects.CIFixSkills(project) }

// CIFixCaps are the configured attempts per check and per PR; zero means the default.
func (h prWatchHost) CIFixCaps() (perCheck, perPR int) {
	set := h.s.settings.Load()
	def := config.DefaultPRConfig()
	return cmp.Or(set.CIFixAttemptsPerCheck, def.CIFixAttemptsPerCheck), cmp.Or(set.CIFixAttemptsPerPR, def.CIFixAttemptsPerPR)
}

func (h prWatchHost) FileFlakyFollowUp(project string, from tickets.Address, f ticketwrite.FollowUp) error {
	dir, err := h.s.projects.Dir(project)
	if err != nil {
		return err
	}
	_, err = recovery.WriteFollowUpIn(filepath.Join(dir, "follow-ups"), from, f)
	return err
}

func (h prWatchHost) QueueRoot(root queue.RootRef, ticketID string) error {
	return h.s.merger.EnsureQueued(root, ticketID)
}

func (h prWatchHost) Env() []string { return h.s.cfg.Env }

func (h prWatchHost) NoChecksGrace() time.Duration { return h.s.settings.Load().NoChecksGrace }

func (h prWatchHost) RepoDir(project string) (string, bool) {
	pf, err := config.ReadProjectFile(filepath.Join(h.s.cfg.TicketStore, project))
	if err != nil || pf.Repo == nil {
		return "", false
	}
	return *pf.Repo, true
}
