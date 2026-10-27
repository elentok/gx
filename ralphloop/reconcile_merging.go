package ralphloop

import (
	"errors"
	"fmt"
	"slices"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

// interruptedMergingReason marks the cherry-picked event startup writes for a
// merging git finished but gx never recorded.
const interruptedMergingReason = "interrupted merging recorded on restart"

// recoverInterruptedMerging handles a merge lock a dead gx left behind. A
// conflicted merging has two halves: the resolver agent finishes the
// cherry-pick itself, then gx records it (cherry-picked event, ticket
// trailer). If gx stopped between the two, the commit is on the feature
// branch but nothing says whose it is, and re-merging it conflicts with
// itself. So when the dead lock's ticket matches exactly the commits the
// feature branch gained since the lock was taken, those commits are recorded
// and stamped as that ticket's merging; the ticket's own reconcile then finds
// it already applied. Either way the dead lock is released, since nothing
// holds it any more. A lock with a marker is a human's pending conflict and a
// live owner is still merging, so both are left alone.
func recoverInterruptedMerging(d Deps, rp reconcileParams, epic tickets.Epic) error {
	lockDir, err := d.mergeLockDir(rp.Paths.ScratchDir, epic.Name)
	if err != nil {
		return fmt.Errorf("resolving merge lock dir: %w", err)
	}
	owner, err := OrphanMergeLock(lockDir)
	if err != nil {
		return fmt.Errorf("reading merge lock: %w", err)
	}
	if owner == nil || owner.Alive() {
		return nil
	}
	inProgress, err := d.CherryPickInProgress(rp.Paths.FeatureWorktree)
	if err != nil {
		return fmt.Errorf("checking for a cherry-pick in progress: %w", err)
	}
	// A cherry-pick still in progress is the merging's first half, not yet
	// done: the ticket's own reconcile reattaches its resolver or aborts it.
	if !inProgress {
		if err := recordInterruptedMerging(d, rp, epic, *owner); err != nil {
			return err
		}
	}
	if err := ReleaseMergeLock(lockDir); err != nil {
		return fmt.Errorf("releasing dead merge lock (%s): %w", owner.Describe(), err)
	}
	return nil
}

// recordInterruptedMerging stamps and records owner's ticket's merging if the
// feature branch's commits since owner.PrePickHead are exactly that ticket's
// iteration commits (same subjects, same order). Anything else (no pre-pick
// head, no new commits, a missing branch, different commits) records nothing.
func recordInterruptedMerging(d Deps, rp reconcileParams, epic tickets.Epic, owner MergeLockOwner) error {
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
	merged, err := d.CommitSubjects(fw, owner.PrePickHead, head)
	if err != nil {
		return fmt.Errorf("listing commits merged since %s: %w", owner.PrePickHead, err)
	}
	picked, err := d.CommitSubjects(fw, base, branch)
	if err != nil {
		return fmt.Errorf("listing %s's commits: %w", branch, err)
	}
	if len(picked) == 0 || !slices.Equal(merged, picked) {
		return nil
	}

	p := rp.iterationParamsFor(epic.Name, t)
	lp := mergeParamsFor(p, base, branch, "")
	if evs, ok, err := ReadEvents(p.ScratchDir, p.FeatureBranch); err == nil && ok {
		if sid, cwd, agent, ok := lastIterationSession(evs, t.Identifier); ok {
			lp.Session = MergeSession{Agent: agent, Cwd: cwd, ID: sid}
		}
	}
	res, err := stampMerged(mergeDepsFor(d), lp)
	if err != nil {
		return fmt.Errorf("stamping interrupted merging of %s: %w", t.Identifier, err)
	}
	p.logTicketEventSHA(string(events.CherryPicked), "", "", fw, interruptedMergingReason, res.SHA)
	return nil
}

// withMergeLock runs merge holding the epic's merge lock, the same lock the
// merge-queue worker and `gx server tickets merge` take, recording the pre-pick HEAD
// so a merging cut short can be finished on restart.
func withMergeLock(d Deps, p iterationParams, merge func() error) error {
	lockDir, err := d.mergeLockDir(p.ScratchDir, p.FeatureBranch)
	if err != nil {
		return err
	}
	head, _ := d.RevParse(p.FeatureWorktree, "HEAD")
	if err := AcquireMergeLockAt(lockDir, p.FeatureBranch, p.Ticket.Identifier, head); err != nil {
		if errors.Is(err, ErrMergeLocked) {
			if owner, readErr := ReadMergeLock(lockDir); readErr == nil && owner != nil {
				return fmt.Errorf("%w by %s", err, owner.Describe())
			}
		}
		return fmt.Errorf("taking merge lock: %w", err)
	}
	defer ReleaseMergeLock(lockDir)
	return merge()
}

// iterationParamsFor is the iterationParams a startup merging of t runs
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
