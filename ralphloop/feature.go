package ralphloop

import (
	"cmp"
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets/schema"
)

// PrepareFeature runs a ticket in the epic's feature worktree itself, on the
// epic branch, with no iteration branch. A missing worktree is recreated from
// the branch. The handle is kept, so ending the ticket only stops its agent.
func PrepareFeature(d Deps, o OneIteration) (IterationWorktree, error) {
	w, err := ResumeFeature(d, o)
	if err != nil {
		return IterationWorktree{}, err
	}
	err = withWorktreeLock(func() error { return addFeatureWorktree(d, o, w.Path) })
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("creating feature worktree for branch %q: %w", o.Epic, err)
	}
	if err := installDepsIn(d, o, w.Path); err != nil {
		return IterationWorktree{}, err
	}
	return w, nil
}

// ResumeFeature rebuilds the handle PrepareFeature made, without touching git.
func ResumeFeature(d Deps, o OneIteration) (IterationWorktree, error) {
	wtDir, err := d.WorktreeDir(o.RepoDir)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving worktree directory for %q: %w", o.RepoDir, err)
	}
	path := filepath.Join(wtDir, o.Epic)
	return IterationWorktree{
		Path: path, Label: iterLabel(o.Epic, o.Ticket.Identifier),
		featureWorktree: path, worktreeDir: wtDir, keep: true,
	}, nil
}

// FinishMerge settles a merge ticket whose agent has gone idle. A paused (answer)
// report pauses as for any ticket. Otherwise it is done only once the epic can
// fast-forward onto target: no rebase in progress in the feature worktree, and
// target an ancestor of the epic. Commits on the epic (a fixer's) are not
// counted; they merge with the epic's fast-forward. Anything else pauses
// paused (repair). The feature worktree stays: the post-merge removal, once the
// epic is merged, is the one place gx removes it.
func FinishMerge(d Deps, o OneIteration, w IterationWorktree, pane, target string) (FinishOutcome, error) {
	p := iterationParams{
		RepoDir: o.RepoDir, FeatureBranch: o.Epic, Agent: o.Agent, Runner: runnerName(d), Label: w.Label,
		Ticket: o.Ticket, ScratchDir: o.ScratchDir, Sink: o.sink(),
	}
	sessionID := recordLiveSession(d, w.Label, o.Ticket.Path)
	adopted, err := adoptAskingReport(p, w.Path, pane, sessionID)
	if err != nil {
		return FinishOutcome{}, err
	}
	if !adopted {
		kind, fault, err := MergeFinishFault(d, o, w.Path, target)
		if err != nil {
			return FinishOutcome{}, err
		}
		if fault != "" {
			reason, err := PauseTicket(p.Sink, PauseRequest{
				ScratchDir: p.ScratchDir, EpicName: p.FeatureBranch, Ticket: p.Ticket.Identifier, Path: p.Ticket.Path,
				Type: events.Paused, Kind: kind, Reason: fault,
				Event: p.agentEvent(pane, sessionID, w.Path),
			})
			if err != nil {
				return FinishOutcome{}, fmt.Errorf("pausing ticket for repair: %w", err)
			}
			return FinishOutcome{Status: schema.StatusLegacyRepair, Kind: kind, Reason: reason}, nil
		}
		if adopted, err = adoptCommitlessFinish(d, p, w.Path, pane, sessionID); err != nil {
			return FinishOutcome{}, err
		}
		if !adopted {
			return FinishOutcome{}, fmt.Errorf("merge ticket %s passed its finish check but was not marked done", o.Ticket.Identifier)
		}
	}
	if err := cleanupCommitless(d, o, w, commitlessSession(w, pane)); err != nil {
		return FinishOutcome{}, err
	}
	t, err := schema.ParseTicket(o.Ticket.Path)
	if err != nil {
		return FinishOutcome{}, fmt.Errorf("reading finished ticket: %w", err)
	}
	if t.Status == schema.StatusDone {
		return FinishOutcome{Merged: true}, nil
	}
	return pausedOutcome(o, t), nil
}

// MergeFinishFault is why a merge ticket cannot be done and the kind it pauses
// with, "" when it can be done.
func MergeFinishFault(d Deps, o OneIteration, dir, target string) (events.Kind, string, error) {
	t, err := schema.ParseTicket(o.Ticket.Path)
	if err != nil {
		return "", "", fmt.Errorf("reading ticket %s for its finish check: %w", o.Ticket.Path, err)
	}
	if t.IterationStatus != schema.IterationStatusFinished {
		return events.MergeUnreported, "merge agent stopped without reporting finished or asking", nil
	}
	rebasing, err := d.RebaseInProgress(dir)
	if err != nil {
		return "", "", fmt.Errorf("checking for a rebase in %s: %w", dir, err)
	}
	if rebasing {
		return events.IterationError, fmt.Sprintf("merge agent reported finished with a rebase still in progress in %s; finish or abort it", dir), nil
	}
	// The tip the target had at claim is enough: trunk may have moved since, and
	// the land step forks a retry merge ticket for that.
	required := target
	if tip := claimTargetTip(o); tip != "" {
		required = tip
	}
	ok, err := d.IsAncestor(dir, required, o.Epic)
	if err != nil {
		return "", "", fmt.Errorf("checking %s is an ancestor of %s: %w", required, o.Epic, err)
	}
	if !ok {
		return events.IterationError, fmt.Sprintf("merge agent reported finished but %s is not an ancestor of %s; rebase it onto %s", required, o.Epic, target), nil
	}
	return "", "", nil
}

// claimTargetTip is the merge target SHA recorded when the ticket was last
// claimed, "" when the run log has none (a ticket claimed before it was kept).
func claimTargetTip(o OneIteration) string {
	evs, _, err := runlog.ReadEvents(o.ScratchDir, o.Epic)
	if err != nil {
		return ""
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if ev := evs[i]; ev.Ticket == o.Ticket.Identifier && ev.Type == string(events.IterationStarted) && ev.TargetTip != "" {
			return ev.TargetTip
		}
	}
	return ""
}

// addFeatureWorktree creates the epic's feature worktree (see Deps.AddWorktree).
// When it attaches a missing worktree to a branch that already exists, as for
// a design's <map>-impl branch cut when the design ended, and the branch lacks
// the base's newer commits, it rebases the branch onto the base first, so the
// epic does not start from a stale tree. Callers hold the worktree lock.
// A conflicting or failing rebase is skipped: the branch keeps its old tip and
// the merge step rebases it later.
func addFeatureWorktree(d Deps, o OneIteration, path string) error {
	exists, err := d.WorktreeExists(path)
	if err != nil {
		return err
	}
	preexisting := !exists && branchExists(d, o.RepoDir, o.Epic)
	if err := d.AddWorktree(o.RepoDir, path, o.Epic, o.RootBase); err != nil {
		return err
	}
	if !preexisting || d.RebaseOntoOrAbort == nil {
		return nil
	}
	base, err := d.RevParse(o.RepoDir, cmp.Or(o.RootBase, "HEAD"))
	if err != nil {
		return nil
	}
	if behind, err := d.IsAncestor(path, base, o.Epic); err != nil || behind {
		return nil
	}
	if changes, err := d.UncommittedChanges(path); err != nil || len(changes) > 0 {
		return nil
	}
	_ = d.RebaseOntoOrAbort(path, base)
	return nil
}
