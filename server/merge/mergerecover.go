package merge

import (
	"fmt"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"path/filepath"

	rlmerge "github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/ralphloop/verify"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/tickets"
)

// RecoverMerges runs before anything is scheduled. A merge lock whose owner is
// dead and that has no conflict marker means a merge was in flight when a
// process died: clear the lock, then let verify say whether the ticket merged.
func (s *Merger) RecoverMerges() {
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

func (s *Merger) recoverEpicMerge(projectDir, epicPath string) error {
	lockDir, err := s.projects.MergeLockDir(epicPath)
	if err != nil {
		return err
	}
	owner, err := rlmerge.OrphanMergeLock(lockDir)
	if err != nil || owner == nil || owner.Alive() {
		return err
	}
	if err := rlmerge.ReleaseMergeLock(lockDir); err != nil {
		return fmt.Errorf("clearing orphan merge lock: %w", err)
	}
	s.log.Info("cleared orphan merge lock", "epic", epicPath, "owner", owner.Describe())
	if owner.Ticket == "" {
		return nil
	}

	project := tickets.ProjectName(projectDir)
	_, repo, err := s.projects.Of(project)
	if err != nil {
		return err
	}
	run, err := repair.DefaultVerifyRun(epicPath, repo, s.host.HostingDeps(tickets.Address{Project: project, Epic: filepath.Base(epicPath), ID: owner.Ticket}).Runner)
	if err != nil {
		return err
	}
	if run.MergeLockStateDir, err = s.projects.LockStateDir(); err != nil {
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
	case verify.MergingMerged:
		ev := runlog.Event{Outcome: string(rlmerge.Merged), SHA: v.SHA}
		if err := tickets.EndOpenRound(t.Path, tickets.SourceGx, tickets.HowManualMerge, "manual-merge"); err != nil {
			return fmt.Errorf("end pause round of %s: %w", addr, err)
		}
		if err := ticketops.RecordManualMerge(projectDir, epic, t.Identifier, t.Path, t.IsDone(), ev); err != nil {
			return fmt.Errorf("finishing merge of %s: %w", addr, err)
		}
		s.events.Publish(wire.EventTicketDone, addr)
	case verify.MergingRecoverable:
		if err := s.abortHalfPick(run.FeatureWorktree); err != nil {
			return fmt.Errorf("rolling back merge of %s: %w", addr, err)
		}
		s.events.Publish(wire.EventMergeRolledBack, addr)
	default:
		reason := fmt.Sprintf("merge interrupted by a crash (lock owner %s) and verify cannot tell whether it merged: %s", owner.Describe(), v.Merging)
		if err := s.writer.Pause(projectDir, tickets.Address{Project: project, Epic: epic, ID: t.Identifier}, t.Path, events.AmbiguousMerge, reason, runs.Run{}); err != nil {
			return fmt.Errorf("pausing %s: %w", addr, err)
		}
	}
	return nil
}

// abortHalfPick drops a cherry-pick the crashed merge left in progress.
func (s *Merger) abortHalfPick(featureWorktree string) error {
	d := s.projects.DefaultDeps()
	inProgress, err := d.CherryPickInProgress(featureWorktree)
	if err != nil || !inProgress {
		return err
	}
	return d.AbortCherryPick(featureWorktree)
}
