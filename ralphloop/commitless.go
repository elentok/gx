package ralphloop

import (
	"fmt"
	"os"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets/schema"
)

// PrepareCommitless gives a commitless one-off somewhere to run that is never
// the live checkout and leaves no branch. With a ref it is a detached worktree
// at that ref; with ref "" (a project without a VCS) it is scratchDir, created
// if missing and kept after the ticket ends.
func PrepareCommitless(d Deps, o OneIteration, scratchDir, ref string) (IterationWorktree, error) {
	label := iterLabel(o.Epic, o.Ticket.Identifier)
	if ref == "" {
		if err := os.MkdirAll(scratchDir, 0o755); err != nil {
			return IterationWorktree{}, fmt.Errorf("creating scratch directory: %w", err)
		}
		return IterationWorktree{Path: scratchDir, Label: label, keep: true}, nil
	}
	wtDir, err := d.WorktreeDir(o.RepoDir)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving worktree directory for %q: %w", o.RepoDir, err)
	}
	path := iterationWorktreePath(wtDir, o.Epic, o.Ticket.Identifier)
	worktreeLock.Lock()
	defer worktreeLock.Unlock()
	if err := d.AddDetachedWorktree(o.RepoDir, path, ref); err != nil {
		return IterationWorktree{}, fmt.Errorf("creating detached worktree at %s: %w", ref, err)
	}
	return IterationWorktree{Path: path, Label: label, worktreeDir: wtDir}, nil
}

// ResumeCommitless rebuilds the handle PrepareCommitless made, for a server that
// restarted mid-iteration. scratchDir is "" for a detached worktree.
func ResumeCommitless(d Deps, o OneIteration, scratchDir string) (IterationWorktree, error) {
	label := iterLabel(o.Epic, o.Ticket.Identifier)
	if scratchDir != "" {
		return IterationWorktree{Path: scratchDir, Label: label, keep: true}, nil
	}
	wtDir, err := d.WorktreeDir(o.RepoDir)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving worktree directory for %q: %w", o.RepoDir, err)
	}
	return IterationWorktree{Path: iterationWorktreePath(wtDir, o.Epic, o.Ticket.Identifier), Label: label, worktreeDir: wtDir}, nil
}

// DiscardCommitless takes back what PrepareCommitless made for a launch that
// failed. A scratch directory stays: nothing in it is the launch's to delete.
func DiscardCommitless(d Deps, o OneIteration, w IterationWorktree) error {
	if w.keep {
		return nil
	}
	return finishCleanup(d, &worktreeLock, o.RepoDir, "", w.Path, "", "", false)
}

// FinishCommitless settles a commitless one-off whose agent has gone idle: an
// agent that reported finished is marked done, a needs-answer report parks, and
// anything else parks as a zero-commit finish. Every outcome but the last
// takes the worktree away; the tab always closes once the ticket is done.
func FinishCommitless(d Deps, o OneIteration, w IterationWorktree, pane, tab string) (FinishOutcome, error) {
	p := iterationParams{
		RepoDir: o.RepoDir, FeatureBranch: o.Epic, Agent: o.Agent, Ticket: o.Ticket,
		ScratchDir: o.ScratchDir, Sink: noopEventSink{},
	}
	sessionID := recordLiveSession(d, w.Label, o.Ticket.Path)
	adopted, err := adoptNeedsAnswerReport(p, w.Path, pane, tab, sessionID)
	if err == nil && !adopted {
		adopted, err = adoptCommitlessFinish(p, w.Path, pane, tab, sessionID)
	}
	if err != nil {
		return FinishOutcome{}, err
	}
	if !adopted {
		if _, err := p.parkNeedsAnswer(events.ZeroCommit, "no result reported", pane, tab, sessionID, w.Path); err != nil {
			return FinishOutcome{}, fmt.Errorf("marking ticket needs-answer: %w", err)
		}
	} else if err := cleanupCommitless(d, o, w, tab); err != nil {
		return FinishOutcome{}, err
	}
	t, err := schema.ParseTicket(o.Ticket.Path)
	if err != nil {
		return FinishOutcome{}, fmt.Errorf("reading finished ticket: %w", err)
	}
	if t.Status == schema.StatusDone {
		return FinishOutcome{Landed: true}, nil
	}
	return FinishOutcome{Status: t.Status, Kind: events.Kind(t.ParkKind)}, nil
}

func cleanupCommitless(d Deps, o OneIteration, w IterationWorktree, tab string) error {
	if !w.keep {
		return finishCleanup(d, &worktreeLock, o.RepoDir, "", w.Path, "", tab, false)
	}
	if tab == "" {
		return nil
	}
	if err := d.TabClose(tab); err != nil {
		return fmt.Errorf("closing iteration tab: %w", err)
	}
	return nil
}
