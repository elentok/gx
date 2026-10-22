package server

import (
	"fmt"
	"github.com/elentok/gx/events"
	"path/filepath"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/tickets"
)

// EventMergeRolledBack streams when startup recovery found a merge that never
// reached the feature branch and undid what it left half done.
const EventMergeRolledBack = "land-rolled-back"

// recoverMerges runs before anything is scheduled. A merge lock whose owner is
// dead and that has no conflict marker means a merge was in flight when a
// process died: clear the lock, then let verify say whether the ticket merged.
func (s *Server) recoverMerges() {
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		s.log.Warn("recover merges", "err", err)
		return
	}
	for _, dir := range dirs {
		epics, err := tickets.Load(dir)
		if err != nil {
			s.log.Warn("recover merges", "project", dir, "err", err)
			continue
		}
		for _, e := range epics {
			if err := s.recoverEpicMerge(dir, e.Path); err != nil {
				s.log.Warn("recover merge", "epic", e.Path, "err", err)
			}
		}
	}
}

func (s *Server) recoverEpicMerge(projectDir, epicPath string) error {
	lockDir, err := ralphloop.MergeLockDir(epicPath)
	if err != nil {
		return err
	}
	owner, err := ralphloop.OrphanMergeLock(lockDir)
	if err != nil || owner == nil || owner.Alive() {
		return err
	}
	if err := ralphloop.ReleaseMergeLock(lockDir); err != nil {
		return fmt.Errorf("clearing orphan merge lock: %w", err)
	}
	s.log.Info("cleared orphan merge lock", "epic", epicPath, "owner", owner.Describe())
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
	s.log.Info("verified interrupted merge", "ticket", addr, "merging", v.Merging, "evidence", v.Evidence)
	switch v.Merging {
	case ralphloop.MergingMerged:
		ev := ralphloop.Event{Outcome: string(ralphloop.Merged), SHA: v.SHA}
		if err := ralphloop.RecordManualMerge(projectDir, epic, t.Identifier, t.Path, t.IsDone(), ev); err != nil {
			return fmt.Errorf("finishing merge of %s: %w", addr, err)
		}
		s.events.publish(EventTicketDone, addr)
	case ralphloop.MergingRecoverable:
		if err := abortHalfPick(run.FeatureWorktree); err != nil {
			return fmt.Errorf("rolling back merge of %s: %w", addr, err)
		}
		s.events.publish(EventMergeRolledBack, addr)
	default:
		reason := fmt.Sprintf("merge interrupted by a crash (lock owner %s) and verify cannot tell whether it merged: %s", owner.Describe(), v.Merging)
		if err := s.pauseTicket(projectDir, tickets.Address{Project: project, Epic: epic, ID: t.Identifier}, t.Path, events.AmbiguousMerge, reason, Run{}); err != nil {
			return fmt.Errorf("pausing %s: %w", addr, err)
		}
	}
	return nil
}

// abortHalfPick drops a cherry-pick the crashed merge left in progress.
func abortHalfPick(featureWorktree string) error {
	d := ralphloop.DefaultDeps()
	inProgress, err := d.CherryPickInProgress(featureWorktree)
	if err != nil || !inProgress {
		return err
	}
	return d.AbortCherryPick(featureWorktree)
}
