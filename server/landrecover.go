package server

import (
	"fmt"
	"github.com/elentok/gx/events"
	"path/filepath"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/tickets"
)

// EventLandRolledBack streams when startup recovery found a land that never
// reached the feature branch and undid what it left half done.
const EventLandRolledBack = "land-rolled-back"

// recoverLands runs before anything is scheduled. A land lock whose owner is
// dead and that has no conflict marker means a land was in flight when a
// process died: clear the lock, then let verify say whether the ticket landed.
func (s *Server) recoverLands() {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		s.log.Warn("recover lands", "err", err)
		return
	}
	for _, dir := range dirs {
		epics, err := tickets.Load(dir)
		if err != nil {
			s.log.Warn("recover lands", "project", dir, "err", err)
			continue
		}
		for _, e := range epics {
			if err := s.recoverEpicLand(dir, e.Path); err != nil {
				s.log.Warn("recover land", "epic", e.Path, "err", err)
			}
		}
	}
}

func (s *Server) recoverEpicLand(projectDir, epicPath string) error {
	lockDir, err := ralphloop.LandLockDir(epicPath)
	if err != nil {
		return err
	}
	owner, err := ralphloop.OrphanLandLock(lockDir)
	if err != nil || owner == nil || owner.Alive() {
		return err
	}
	if err := ralphloop.ReleaseLandLock(lockDir); err != nil {
		return fmt.Errorf("clearing orphan land lock: %w", err)
	}
	s.log.Info("cleared orphan land lock", "epic", epicPath, "owner", owner.Describe())
	if owner.Ticket == "" {
		return nil
	}

	project := tickets.ProjectName(projectDir)
	_, repo, err := s.projectOf(project)
	if err != nil {
		return err
	}
	run, err := repair.DefaultVerifyRun(epicPath, repo)
	if err != nil {
		return err
	}
	run.ID = owner.Ticket
	res, err := repair.Verify(run)
	if err != nil {
		return err
	}
	_, t, err := repair.FindEpicTicket(epicPath, owner.Ticket)
	if err != nil {
		return err
	}
	epic := filepath.Base(epicPath)
	addr := tickets.Address{Project: project, Epic: epic, ID: t.Identifier}.String()

	v := res.Tickets[0]
	s.log.Info("verified interrupted land", "ticket", addr, "landing", v.Landing, "evidence", v.Evidence)
	switch v.Landing {
	case ralphloop.LandingLanded:
		ev := ralphloop.Event{Outcome: string(ralphloop.Landed), SHA: v.SHA}
		if err := ralphloop.RecordManualLand(projectDir, epic, t.Identifier, t.Path, t.IsDone(), ev); err != nil {
			return fmt.Errorf("finishing land of %s: %w", addr, err)
		}
		s.events.publish(EventTicketDone, addr)
	case ralphloop.LandingRecoverable:
		if err := abortHalfPick(run.FeatureWorktree); err != nil {
			return fmt.Errorf("rolling back land of %s: %w", addr, err)
		}
		s.events.publish(EventLandRolledBack, addr)
	default:
		reason := fmt.Sprintf("land interrupted by a crash (lock owner %s) and verify cannot tell whether it landed: %s", owner.Describe(), v.Landing)
		if err := s.parkTicket(projectDir, tickets.Address{Project: project, Epic: epic, ID: t.Identifier}, t.Path, events.AmbiguousLand, reason); err != nil {
			return fmt.Errorf("parking %s: %w", addr, err)
		}
	}
	return nil
}

// abortHalfPick drops a cherry-pick the crashed land left in progress.
func abortHalfPick(featureWorktree string) error {
	d := ralphloop.DefaultDeps()
	inProgress, err := d.CherryPickInProgress(featureWorktree)
	if err != nil || !inProgress {
		return err
	}
	return d.AbortCherryPick(featureWorktree)
}
