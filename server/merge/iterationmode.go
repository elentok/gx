package merge

import (
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// IterationMode is where a ticket runs and how its iteration ends: on the epic's
// feature branch, commitless (a prompt ticket, or anything in the scratch
// project, which has no VCS), or in the feature worktree itself (a merge
// ticket). Decided once per ticket, so launch, finish and resume cannot disagree.
type IterationMode struct {
	Commitless bool
	// Feature runs in the epic's Feature worktree with no iteration branch, and
	// ends like a commitless ticket on a kept directory: only the agent stops.
	Feature bool
	// target is where a feature-mode epic merges, resolved at finish so the check
	// sees the trunk current then.
	target func() (string, error)
	// ScratchDir is a kept directory the agent runs in directly, with no worktree:
	// the scratch dir of a scratch-project ticket, or the target checkout of a
	// trunk-conflict ticket.
	ScratchDir string
	// err is why a trunk-conflict ticket's checkout could not be found; prepare
	// and resume fail with it instead of running the agent somewhere else.
	err error
	// finishCheck, when set, runs before a commitless ticket closes and may pause
	// it instead; a trunk-conflict ticket scans its files for conflict markers.
	// A pr-text ticket cancels it instead.
	finishCheck func(d deps.Deps, o ralphloop.OneIteration, w ralphloop.IterationWorktree, pane string) (out ralphloop.FinishOutcome, handled bool, err error)
	// AfterDone, when set, runs once the ticket's finished report is adopted.
	AfterDone func()
	// Ref is the detached worktree's start for a commitless ticket in a real
	// project; launch-only, so resume never needs it.
	Ref string
}

func (s *Landing) IterationModeFor(addr tickets.Address, t tickets.Ticket) IterationMode {
	typ := schema.TicketType(t.Type)
	if typ == schema.TypeMerge || prflow.RunsInFeature(typ) {
		return IterationMode{Feature: true, target: func() (string, error) { return s.host.MergeTarget(addr) }}
	}
	if typ == schema.TypeTrunkConflict {
		dir, err := s.targetCheckout(addr)
		m := IterationMode{Commitless: true, ScratchDir: dir, err: err, AfterDone: func() { s.dropTrunkConflictBackup(addr) }}
		// An epic with no recorded files still gets the scan.
		var files []string
		if e, ok := s.writer.FreshEpic(addr.Project, addr.Epic); ok {
			files = e.TrunkConflictFiles
		}
		m.finishCheck = func(_ deps.Deps, o ralphloop.OneIteration, _ ralphloop.IterationWorktree, _ string) (ralphloop.FinishOutcome, bool, error) {
			return pauseOnMarkers(o, dir, files)
		}
		return m
	}
	if typ == schema.TypePRText {
		return s.prTextMode(addr)
	}
	m := IterationMode{Commitless: addr.Project == projects.ScratchProject || addr.Epic == queue.OneOffsEpic || typ == schema.TypePrompt || typ == schema.TypeInvestigate || typ == schema.TypeEvolve || typ == schema.TypeDiagnose}
	if m.Commitless && (addr.Project == projects.ScratchProject || s.projectHasNoVCS(addr.Project)) {
		m.ScratchDir = projects.ScratchSubdir(s.cfg.TicketStore, addr.Epic)
	}
	return m
}

// prTextMode runs a pr-text ticket commitless in the epic's feature worktree,
// which it only reads. A Resolution that fails the check cancels the attempt and
// has prflow fork the next, or store pr-text-failed after the last.
func (s *Landing) prTextMode(addr tickets.Address) IterationMode {
	m := IterationMode{Commitless: true, ScratchDir: filepath.Join(s.projects.WorktreeDir(addr.Project), addr.Epic)}
	m.finishCheck = func(d deps.Deps, o ralphloop.OneIteration, w ralphloop.IterationWorktree, pane string) (ralphloop.FinishOutcome, bool, error) {
		t, err := schema.ParseTicket(o.Ticket.Path)
		if err != nil || t.IterationStatus != schema.IterationStatusFinished {
			return ralphloop.FinishOutcome{}, false, err
		}
		reason, err := prflow.CheckText(o.Ticket.Path)
		if err != nil || reason == "" {
			return ralphloop.FinishOutcome{}, false, err
		}
		out, err := ralphloop.CancelCommitless(d, o, w, pane, reason)
		if err != nil {
			return out, false, err
		}
		epic, ok := s.writer.FreshEpic(addr.Project, addr.Epic)
		if !ok {
			return out, true, fmt.Errorf("epic %s not found", addr.Epic)
		}
		if _, err := prflow.AfterInvalidText(epic.Path, o.Ticket.Path, reason); err != nil {
			return out, true, fmt.Errorf("after invalid PR text: %w", err)
		}
		s.events.Publish(wire.EventQueueChanged, queue.RootOf(addr).String())
		return out, true, nil
	}
	return m
}

// targetCheckout is the directory where addr's root epic merges: the checkout
// of its target branch, where the uncommitted files a trunk-conflict ticket
// resolves live.
func (s *Landing) targetCheckout(addr tickets.Address) (string, error) {
	target, err := s.host.MergeTarget(addr)
	if err != nil {
		return "", err
	}
	_, repo, err := s.projects.Of(addr.Project)
	if err != nil {
		return "", err
	}
	_, dir, err := checkoutDirFor(repo, target)
	return dir, err
}

// checkoutDirFor is the repo and the working tree that has target checked out;
// an error when none has.
func checkoutDirFor(repoDir, target string) (git.Repo, string, error) {
	info, err := git.IdentifyDir(repoDir)
	if err != nil {
		return git.Repo{}, "", err
	}
	worktrees, err := git.ListWorktrees(info.Repo)
	if err != nil {
		return git.Repo{}, "", err
	}
	dir := history.CheckoutDirForBranch(info.Repo, worktrees, target)
	if dir == "" {
		return git.Repo{}, "", fmt.Errorf("no checkout of %s", target)
	}
	return info.Repo, dir, nil
}

// projectHasNoVCS: a commitless ticket in a vcs: none project has no repository
// to check out, so it runs in a scratch subdir.
func (s *Landing) projectHasNoVCS(project string) bool {
	dir, err := s.projects.Dir(project)
	if err != nil {
		return false
	}
	pf, _ := config.ReadProjectFile(dir)
	return pf.VCS != nil && *pf.VCS == config.VCSNone
}

func (m IterationMode) Prepare(d deps.Deps, o ralphloop.OneIteration) (ralphloop.IterationWorktree, error) {
	switch {
	case m.err != nil:
		return ralphloop.IterationWorktree{}, m.err
	case m.Feature:
		return ralphloop.PrepareFeature(d, o)
	case m.Commitless:
		return ralphloop.PrepareCommitless(d, o, m.ScratchDir, m.Ref)
	}
	return ralphloop.PrepareIteration(d, o)
}

func (m IterationMode) Discard(d deps.Deps, o ralphloop.OneIteration, w ralphloop.IterationWorktree) error {
	if m.Commitless || m.Feature {
		return ralphloop.DiscardCommitless(d, o, w)
	}
	return ralphloop.DiscardIteration(d, o, w)
}

func (m IterationMode) Finish(d deps.Deps, o ralphloop.OneIteration, w ralphloop.IterationWorktree, pane string) (ralphloop.FinishOutcome, error) {
	switch {
	case m.Feature:
		target, err := m.target()
		if err != nil {
			return ralphloop.FinishOutcome{}, fmt.Errorf("resolving merge target: %w", err)
		}
		return ralphloop.FinishMerge(d, o, w, pane, target)
	case m.Commitless:
		if m.finishCheck != nil {
			if out, handled, err := m.finishCheck(d, o, w, pane); err != nil || handled {
				return out, err
			}
		}
		return ralphloop.FinishCommitless(d, o, w, pane)
	}
	return ralphloop.FinishIteration(d, o, w, pane)
}

// pauseOnMarkers is the trunk-conflict ticket's check of an agent's finished
// report: the report alone is not trusted, so files that still hold conflict
// markers pause the ticket instead of closing it.
func pauseOnMarkers(o ralphloop.OneIteration, dir string, files []string) (out ralphloop.FinishOutcome, paused bool, err error) {
	t, err := schema.ParseTicket(o.Ticket.Path)
	if err != nil {
		return ralphloop.FinishOutcome{}, false, fmt.Errorf("reading finished ticket: %w", err)
	}
	if t.IterationStatus != schema.IterationStatusFinished {
		return ralphloop.FinishOutcome{}, false, nil
	}
	left := filesWithMarkers(dir, files)
	if len(left) == 0 {
		return ralphloop.FinishOutcome{}, false, nil
	}
	reason := "conflict markers are still in " + listFiles(left)
	kind := events.ConflictMarkers
	if err := ticketops.Pause(o.ScratchDir, o.Epic, o.Ticket.Identifier, o.Ticket.Path, kind, reason, runlog.Event{}); err != nil {
		return ralphloop.FinishOutcome{}, false, fmt.Errorf("pausing on conflict markers: %w", err)
	}
	return ralphloop.FinishOutcome{Status: schema.Status(ticketops.PauseStatus(kind)), Kind: kind, Reason: reason}, true, nil
}

// Resume rebuilds the worktree; base is the persisted base of a feature-branch run.
func (m IterationMode) Resume(d deps.Deps, o ralphloop.OneIteration, base string) (ralphloop.IterationWorktree, error) {
	switch {
	case m.err != nil:
		return ralphloop.IterationWorktree{}, m.err
	case m.Feature:
		return ralphloop.ResumeFeature(d, o)
	case m.Commitless:
		return ralphloop.ResumeCommitless(d, o, m.ScratchDir)
	}
	return ralphloop.ResumeIteration(d, o, base)
}
