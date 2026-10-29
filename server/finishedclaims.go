package server

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// finishedClaimGrace is how long a claimed ticket may sit with its agent's
// finished report before it counts as lost. A live completion settles within
// seconds, so this only catches one the loop never processed.
const finishedClaimGrace = 5 * time.Minute

// reportedFinished reports whether the claimed ticket at path already holds its
// agent's finished report, so a reclaim settles it instead of waiting on an
// agent that may never read as done again.
func reportedFinished(path string) bool {
	t, err := schema.ParseTicket(path)
	return err == nil && t.Status == schema.StatusClaimed && t.IterationStatus == schema.IterationStatusFinished
}

// pauseStuckFinishedClaims pauses each claimed ticket whose finished report is
// older than the grace, so it reaches Needs you instead of blocking its epic.
// Startup reclaim settles the tracked ones; this catches the rest, such as a
// wait that never saw the agent finish.
func (s *Server) pauseStuckFinishedClaims(now time.Time) {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		epics, _ := s.idx.epicsOf(dir)
		for _, e := range epics {
			for _, it := range e.Tickets {
				if schema.Status(it.Status) != schema.StatusClaimed {
					continue
				}
				t, err := schema.ParseTicket(it.Path)
				if err != nil || t.Status != schema.StatusClaimed || t.IterationStatus != schema.IterationStatusFinished ||
					t.IterationFinishedAt.IsZero() || now.Sub(t.IterationFinishedAt) < finishedClaimGrace {
					continue
				}
				addr := tickets.Address{Project: tickets.ProjectName(dir), Epic: filepath.Base(e.Path), ID: it.Identifier}
				if tr, ok := s.registry.tracked(addr.String()); ok && tr.Settling {
					continue
				}
				run, _ := s.registry.runOf(addr.String())
				reason := fmt.Sprintf("agent reported finished at %s but the ticket was never processed", t.IterationFinishedAt.UTC().Format(time.RFC3339))
				if err := s.pauseTicket(dir, addr, it.Path, events.IterationError, reason, run); err != nil {
					s.log.Warn("pause finished claim", "ticket", addr, "err", err)
				}
			}
		}
	}
}
