package ralphloop

import (
	"fmt"
	"os"
	"strings"

	"github.com/elentok/gx/agentrunner"
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
	return finishCleanup(d, &worktreeLock, o.RepoDir, "", w.Path, "", agentrunner.Session{}, false)
}

// FinishCommitless settles a commitless one-off whose agent has gone idle: a
// detached worktree holding commits nothing else reaches parks and stays, an
// agent that reported finished is marked done, a needs-answer report parks, and
// anything else parks as a zero-commit finish. Both reports take the worktree
// away; the tab always closes once the ticket is done.
func FinishCommitless(d Deps, o OneIteration, w IterationWorktree, pane string) (FinishOutcome, error) {
	p := iterationParams{
		RepoDir: o.RepoDir, FeatureBranch: o.Epic, Agent: o.Agent, Ticket: o.Ticket,
		ScratchDir: o.ScratchDir, Sink: o.sink(),
	}
	sessionID := recordLiveSession(d, w.Label, o.Ticket.Path)
	stranded, err := parkStrandedCommits(d, p, w, pane, sessionID)
	if err != nil {
		return FinishOutcome{}, err
	}
	adopted := false
	if !stranded {
		adopted, err = adoptNeedsAnswerReport(p, w.Path, pane, sessionID)
		if err == nil && !adopted {
			adopted, err = adoptCommitlessFinish(p, w.Path, pane, sessionID)
		}
	}
	if err != nil {
		return FinishOutcome{}, err
	}
	switch {
	case stranded:
	case !adopted:
		if _, err := p.parkNeedsAnswer(events.ZeroCommit, "no result reported", pane, sessionID, w.Path); err != nil {
			return FinishOutcome{}, fmt.Errorf("marking ticket needs-answer: %w", err)
		}
	default:
		if err := cleanupCommitless(d, o, w, commitlessSession(w, pane)); err != nil {
			return FinishOutcome{}, err
		}
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

// parkStrandedCommits parks the ticket when w is a detached worktree whose HEAD
// carries commits nothing else reaches: removing it would leave them dangling,
// so the worktree stays until a person lands or drops them. A scratch directory
// has no commits to strand.
func parkStrandedCommits(d Deps, p iterationParams, w IterationWorktree, pane, sessionID string) (bool, error) {
	if w.keep {
		return false, nil
	}
	commits, err := d.UnreachableCommits(w.Path)
	if err != nil {
		return false, fmt.Errorf("listing commits on %s: %w", w.Path, err)
	}
	if len(commits) == 0 {
		return false, nil
	}
	reason := fmt.Sprintf("commitless ticket committed on its detached worktree %s; land or drop: %s", w.Path, strings.Join(commits, "; "))
	if _, err := p.parkNeedsAnswer(events.StrandedCommits, reason, pane, sessionID, w.Path); err != nil {
		return false, fmt.Errorf("marking ticket needs-answer: %w", err)
	}
	return true, nil
}

// commitlessSession is the live session of w's one-off, the zero Session when
// the iteration has no pane.
func commitlessSession(w IterationWorktree, pane string) agentrunner.Session {
	if pane == "" {
		return agentrunner.Session{}
	}
	return agentrunner.Session{Label: w.Label, ID: pane}
}

func cleanupCommitless(d Deps, o OneIteration, w IterationWorktree, s agentrunner.Session) error {
	if !w.keep {
		return finishCleanup(d, &worktreeLock, o.RepoDir, "", w.Path, "", s, false)
	}
	if s == (agentrunner.Session{}) {
		return nil
	}
	if err := d.Runner.Stop(s); err != nil {
		return fmt.Errorf("stopping iteration session: %w", err)
	}
	return nil
}
