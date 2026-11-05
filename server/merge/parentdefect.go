package merge

import (
	"path/filepath"
	"slices"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// ScanParentDefects starts recovery for each lettered ticket whose parent its
// ID does not allow. It runs on every rescan, so a ticket whose run log already
// holds the defect is skipped: a remedy that fixed it leaves nothing to find,
// and one refused or stopped by a guard rail must not be raised again. Done
// tickets are skipped too: their parent no longer changes scheduling, and an
// old epic's backlog would otherwise flood recovery on the first scan. Drafts
// are skipped as unfinished. Nothing is raised while R14 is off: a raised
// defect is never raised again, and unmatched it would only fork investigations.
func (s *Reconciler) ScanParentDefects() {
	if !s.settings.Load().Recovery.EntryEnabled("R14") {
		return
	}
	// A rescan from the watch and one from a ping may overlap; one at a time
	// keeps the run-log check and the append together.
	s.defectScan.Lock()
	defer s.defectScan.Unlock()
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		epics, _ := s.idx.EpicsOf(dir)
		for _, e := range epics {
			if e.Status == string(schema.StatusDraft) {
				continue
			}
			for _, t := range e.Tickets {
				want, defect := recovery.ParentDefect(t.Identifier, t.Parent)
				if defect && t.Status != string(schema.StatusDone) && t.Status != string(schema.StatusDraft) {
					s.raiseParentDefect(dir, filepath.Base(e.Path), t.Identifier, want)
				}
			}
		}
	}
}

func (s *Reconciler) raiseParentDefect(projectDir, epic, id, want string) {
	log, _, err := runlog.ReadEvents(projectDir, epic)
	if err != nil {
		s.log.Warn("parent scan cannot read the run log", "epic", epic, "err", err)
		return
	}
	if slices.ContainsFunc(log, func(ev runlog.Event) bool {
		return ev.Ticket == id && events.Type(ev.Type) == events.TicketGraphDefect && events.Kind(ev.Kind) == events.ParentDefect
	}) {
		return
	}
	ev := runlog.Event{Type: string(events.TicketGraphDefect), Ticket: id, Kind: string(events.ParentDefect), Reason: want}
	if err := runlog.AppendEvent(projectDir, epic, ev); err != nil {
		s.log.Warn("parent scan cannot record event", "epic", epic, "ticket", id, "err", err)
		return
	}
	addr := tickets.Address{Project: tickets.ProjectName(projectDir), Epic: epic, ID: id}
	s.host.RecoverAsync(recovery.Failure{Address: addr.String(), Type: events.TicketGraphDefect, Kind: events.ParentDefect, Reason: want, Parent: want})
}
