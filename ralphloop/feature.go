package ralphloop

import (
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/events"
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
	worktreeLock.Lock()
	defer worktreeLock.Unlock()
	if err := d.AddWorktree(o.RepoDir, w.Path, o.Epic, o.RootBase); err != nil {
		return IterationWorktree{}, fmt.Errorf("creating feature worktree for branch %q: %w", o.Epic, err)
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

// FinishMerge settles a merge ticket whose agent has gone idle. A needs-answer
// report parks as for any ticket. Otherwise it is done only once the epic can
// fast-forward onto target: no rebase in progress in the feature worktree, and
// target an ancestor of the epic. Commits on the epic (a fixer's) are not
// counted; they land with the epic's fast-forward. Anything else parks
// needs-repair. The feature worktree always stays.
func FinishMerge(d Deps, o OneIteration, w IterationWorktree, pane, target string) (FinishOutcome, error) {
	p := iterationParams{
		RepoDir: o.RepoDir, FeatureBranch: o.Epic, Agent: o.Agent, Runner: runnerName(d), Label: w.Label,
		Ticket: o.Ticket, ScratchDir: o.ScratchDir, Sink: o.sink(),
	}
	sessionID := recordLiveSession(d, w.Label, o.Ticket.Path)
	adopted, err := adoptNeedsAnswerReport(p, w.Path, pane, sessionID)
	if err != nil {
		return FinishOutcome{}, err
	}
	if !adopted {
		kind, fault, err := MergeFinishFault(d, o, w.Path, target)
		if err != nil {
			return FinishOutcome{}, err
		}
		if fault != "" {
			reason, err := park(p.Sink, parkRequest{
				ScratchDir: p.ScratchDir, EpicName: p.FeatureBranch, Ticket: p.Ticket.Identifier, Path: p.Ticket.Path,
				Type: events.Paused, Kind: kind, Reason: fault,
				Event: p.agentEvent(pane, sessionID, w.Path),
			})
			if err != nil {
				return FinishOutcome{}, fmt.Errorf("marking ticket needs-repair: %w", err)
			}
			return FinishOutcome{Status: schema.StatusLegacyRepair, Kind: kind, Reason: reason}, nil
		}
		if adopted, err = adoptCommitlessFinish(p, w.Path, pane, sessionID); err != nil {
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
		return FinishOutcome{Landed: true}, nil
	}
	return parkedOutcome(o, t), nil
}

// MergeFinishFault is why a merge ticket cannot be done and the kind it parks
// with, "" when it can be done.
func MergeFinishFault(d Deps, o OneIteration, dir, target string) (events.Kind, string, error) {
	t, err := schema.ParseTicket(o.Ticket.Path)
	if err != nil {
		return "", "", fmt.Errorf("reading ticket %s for its finish check: %w", o.Ticket.Path, err)
	}
	if t.IterationStatus != schema.IterationStatusFinished {
		return events.MergeUnreported, "merge agent stopped without reporting finished or needs-answer", nil
	}
	rebasing, err := d.RebaseInProgress(dir)
	if err != nil {
		return "", "", fmt.Errorf("checking for a rebase in %s: %w", dir, err)
	}
	if rebasing {
		return events.IterationError, fmt.Sprintf("merge agent reported finished with a rebase still in progress in %s; finish or abort it", dir), nil
	}
	ok, err := d.IsAncestor(dir, target, o.Epic)
	if err != nil {
		return "", "", fmt.Errorf("checking %s is an ancestor of %s: %w", target, o.Epic, err)
	}
	if !ok {
		return events.IterationError, fmt.Sprintf("merge agent reported finished but %s is not an ancestor of %s; rebase it onto %s", target, o.Epic, target), nil
	}
	return "", "", nil
}
