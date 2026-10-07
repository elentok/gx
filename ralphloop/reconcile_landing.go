package ralphloop

import (
	"errors"
	"fmt"
	"slices"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

// interruptedLandingReason marks the cherry-picked event startup writes for a
// landing git finished but gx never recorded.
const interruptedLandingReason = "interrupted landing recorded on restart"

// recoverInterruptedLanding handles a land lock a dead gx left behind. A
// conflicted landing has two halves: the resolver agent finishes the
// cherry-pick itself, then gx records it (cherry-picked event, ticket
// trailer). If gx stopped between the two, the commit is on the feature
// branch but nothing says whose it is, and re-landing it conflicts with
// itself. So when the dead lock's ticket matches exactly the commits the
// feature branch gained since the lock was taken, those commits are recorded
// and stamped as that ticket's landing; the ticket's own reconcile then finds
// it already applied. Either way the dead lock is released, since nothing
// holds it any more. A lock with a marker is a human's pending conflict and a
// live owner is still landing, so both are left alone.
func recoverInterruptedLanding(d Deps, rp reconcileParams, epic tickets.Epic) error {
	lockDir := landLockDir(rp.Paths.ScratchDir, epic.Name)
	owner, err := OrphanLandLock(lockDir)
	if err != nil {
		return fmt.Errorf("reading land lock: %w", err)
	}
	if owner == nil || owner.Alive() {
		return nil
	}
	inProgress, err := d.CherryPickInProgress(rp.Paths.FeatureWorktree)
	if err != nil {
		return fmt.Errorf("checking for a cherry-pick in progress: %w", err)
	}
	// A cherry-pick still in progress is the landing's first half, not yet
	// done: the ticket's own reconcile reattaches its resolver or aborts it.
	if !inProgress {
		if err := recordInterruptedLanding(d, rp, epic, *owner); err != nil {
			return err
		}
	}
	if err := ReleaseLandLock(lockDir); err != nil {
		return fmt.Errorf("releasing dead land lock (%s): %w", owner.Describe(), err)
	}
	return nil
}

// recordInterruptedLanding stamps and records owner's ticket's landing if the
// feature branch's commits since owner.PrePickHead are exactly that ticket's
// iteration commits (same subjects, same order). Anything else (no pre-pick
// head, no new commits, a missing branch, different commits) records nothing.
func recordInterruptedLanding(d Deps, rp reconcileParams, epic tickets.Epic, owner LandLockOwner) error {
	if owner.Epic != epic.Name || owner.Ticket == "" || owner.PrePickHead == "" {
		return nil
	}
	i := slices.IndexFunc(epic.Tickets, func(t tickets.Ticket) bool { return t.Identifier == owner.Ticket })
	if i < 0 {
		return nil
	}
	t := epic.Tickets[i]
	fw := rp.Paths.FeatureWorktree
	branch := iterBranch(epic.Name, t.Identifier)
	if !branchExists(d, fw, branch) {
		return nil
	}

	head, err := d.RevParse(fw, "HEAD")
	if err != nil {
		return fmt.Errorf("resolving %s's HEAD: %w", epic.Name, err)
	}
	if head == owner.PrePickHead {
		return nil
	}
	moved, err := d.IsAncestor(fw, owner.PrePickHead, head)
	if err != nil || !moved {
		return err
	}
	base, err := d.MergeBase(fw, branch, owner.PrePickHead)
	if err != nil {
		return fmt.Errorf("resolving %s's base: %w", branch, err)
	}
	landed, err := d.CommitSubjects(fw, owner.PrePickHead, head)
	if err != nil {
		return fmt.Errorf("listing commits landed since %s: %w", owner.PrePickHead, err)
	}
	picked, err := d.CommitSubjects(fw, base, branch)
	if err != nil {
		return fmt.Errorf("listing %s's commits: %w", branch, err)
	}
	if len(picked) == 0 || !slices.Equal(landed, picked) {
		return nil
	}

	p := rp.iterationParamsFor(epic.Name, t)
	lp := landParamsFor(p, base, branch, "")
	if evs, ok, err := ReadEvents(p.ScratchDir, p.FeatureBranch); err == nil && ok {
		if sid, cwd, agent, ok := lastIterationSession(evs, t.Identifier); ok {
			lp.Session = LandSession{Agent: agent, Cwd: cwd, ID: sid}
		}
	}
	res, err := stampLanded(landDepsFor(d), lp)
	if err != nil {
		return fmt.Errorf("stamping interrupted landing of %s: %w", t.Identifier, err)
	}
	p.logTicketEventSHA(string(events.CherryPicked), "", "", "", fw, interruptedLandingReason, res.SHA)
	return nil
}

// withLandLock runs land holding the epic's land lock, the same lock the
// land-queue worker and `gx tickets land` take, recording the pre-pick HEAD
// so a landing cut short can be finished on restart.
func withLandLock(d Deps, p iterationParams, land func() error) error {
	lockDir := landLockDir(p.ScratchDir, p.FeatureBranch)
	head, _ := d.RevParse(p.FeatureWorktree, "HEAD")
	if err := AcquireLandLockAt(lockDir, p.FeatureBranch, p.Ticket.Identifier, head); err != nil {
		if errors.Is(err, ErrLandLocked) {
			if owner, readErr := ReadLandLock(lockDir); readErr == nil && owner != nil {
				return fmt.Errorf("%w by %s", err, owner.Describe())
			}
		}
		return fmt.Errorf("taking land lock: %w", err)
	}
	defer ReleaseLandLock(lockDir)
	return land()
}

// iterationParamsFor is the iterationParams a startup landing of t runs
// under.
func (rp reconcileParams) iterationParamsFor(featureBranch string, t tickets.Ticket) iterationParams {
	return iterationParams{
		WorkspaceID:     rp.WorkspaceID,
		RepoDir:         rp.Paths.RepoDir,
		WorktreeDir:     rp.Paths.WorktreeDir,
		FeatureWorktree: rp.Paths.FeatureWorktree,
		FeatureBranch:   featureBranch,
		Agent:           rp.Agent,
		Model:           rp.Model,
		Effort:          rp.Effort,
		Skill:           rp.Skill,
		Ticket:          t,
		ScratchDir:      rp.Paths.ScratchDir,
		WorktreeLock:    rp.WorktreeLock,
		SmartZone:       rp.SmartZone,
		Gate:            rp.Gate,
		Sink:            rp.Sink,
	}
}
