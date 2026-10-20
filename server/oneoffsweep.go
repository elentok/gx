package server

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// sweepOneOffs moves every done or cancelled ticket of a one-offs epic into
// one-offs-archived, whatever wrote its status. It runs after each rescan, so a
// crash between a move's two writes is finished by the first rescan of the next
// server. A ticket whose run is still tracked stays: the run's post-landing
// steps (retro stamp, result, follow-up) still read it at its old address.
func (s *Server) sweepOneOffs() {
	if s.cfg.TicketStore == "" {
		return
	}
	s.sweepMu.Lock()
	defer s.sweepMu.Unlock()

	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		s.log.Warn("sweep one-offs", "err", err)
		return
	}
	moved := false
	for _, dir := range dirs {
		epics, _ := s.idx.epicsOf(dir)
		for _, e := range epics {
			if filepath.Base(e.Path) != OneOffsEpic {
				continue
			}
			for _, t := range e.Tickets {
				if t.Status != string(schema.StatusDone) && t.Status != string(schema.StatusCancelled) {
					continue
				}
				addr := tickets.Address{Project: tickets.ProjectName(dir), Epic: OneOffsEpic, ID: t.Identifier}
				if _, live := s.registry.tracked(addr.String()); live {
					continue
				}
				if err := archiveOneOff(dir, addr, t, time.Now()); err != nil {
					s.log.Warn("archive one-off", "ticket", addr.String(), "err", err)
					continue
				}
				moved = true
			}
		}
	}
	if moved {
		if err := s.idx.refresh(s.cfg.TicketStore); err != nil {
			s.log.Warn("refresh after one-off sweep", "err", err)
		}
	}
	for _, dir := range dirs {
		s.dequeueIdleOneOffs(dir)
	}
}

// dequeueIdleOneOffs takes the project's one-offs root off the queue once no
// live one-off is left; the next submit queues it again. A submit writes its
// ticket before it looks at the queue, so the disk check under the queue lock
// either sees that ticket (root stays) or the submit finds the root gone and
// queues it afresh.
func (s *Server) dequeueIdleOneOffs(dir string) {
	root := rootRef{Project: tickets.ProjectName(dir), Epic: OneOffsEpic}
	removed, err := s.queued.removeRootIf(root, func() bool {
		return !s.oneOffsLive(dir, root)
	})
	if err != nil {
		s.log.Warn("dequeue idle one-offs", "root", root, "err", err)
	} else if removed {
		s.events.publish(EventQueueChanged, root.String())
	}
}

// oneOffsLive reports whether a one-off is still pending, or its run is tracked.
// Failed, parked and needs-* tickets are not terminal, so they count.
func (s *Server) oneOffsLive(dir string, root rootRef) bool {
	epics, err := tickets.Load(dir)
	if err != nil {
		return true
	}
	for _, e := range epics {
		if filepath.Base(e.Path) != OneOffsEpic {
			continue
		}
		for _, t := range e.Tickets {
			addr := tickets.Address{Project: root.Project, Epic: OneOffsEpic, ID: t.Identifier}
			if _, tracked := s.registry.tracked(addr.String()); tracked {
				return true
			}
			if t.Status != string(schema.StatusDone) && t.Status != string(schema.StatusCancelled) {
				return true
			}
		}
	}
	return false
}

// archiveOneOff moves one terminal one-off into the archive epic, creating the
// epic on the first move. The archive copy is written before the original is
// removed; a copy already there (its Resolution names addr) means an earlier
// move was cut short, so only its leftovers are finished.
func archiveOneOff(projectDir string, addr tickets.Address, t tickets.Ticket, now time.Time) error {
	archivePath := filepath.Join(projectDir, OneOffsArchivedEpic)
	if err := ensureDraftEpic(archivePath); err != nil {
		return err
	}
	unlock, err := tickets.LockEpics(filepath.Join(projectDir, OneOffsEpic), archivePath)
	if err != nil {
		return err
	}
	defer unlock()

	// The index may be a scan behind: only a ticket still terminal on disk moves.
	tk, err := schema.ParseTicket(t.Path)
	if err != nil || (tk.Status != schema.StatusDone && tk.Status != schema.StatusCancelled) {
		return nil
	}
	epics, err := tickets.Load(projectDir)
	if err != nil {
		return err
	}
	for _, e := range epics {
		if filepath.Base(e.Path) != OneOffsArchivedEpic {
			continue
		}
		for _, c := range e.Tickets {
			if resolutionNames(c.Body, addr) {
				to := tickets.Address{Project: addr.Project, Epic: OneOffsArchivedEpic, ID: c.Identifier}
				return tickets.ResumeMove(projectDir, addr, to, t.Path, c.Path)
			}
		}
	}
	completed := tk.FinishedAt
	if completed.IsZero() {
		completed = now.UTC()
	}
	p, err := tickets.PlanMove(projectDir, addr, t, OneOffsArchivedEpic, "", tickets.MoveOpts{Archive: true, AppendOriginal: true, CompletedAt: completed})
	if err != nil {
		return err
	}
	return tickets.ApplyMove(p)
}

// resolutionNames reports whether a ticket body's Resolution records addr as its
// original address.
func resolutionNames(body string, addr tickets.Address) bool {
	want := "- original: " + addr.String()
	for _, line := range strings.Split(schema.Section(body, "Resolution"), "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}
