package ralphloop

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/tickets"
)

// OneIteration is the fixed input of a single-ticket iteration driven from
// outside Run (the server's runner). Label, branch and worktree names come out
// of the same helpers Run uses, so both orchestrators leave identical state.
type OneIteration struct {
	RepoDir     string
	WorkspaceID string
	Epic        string
	ScratchDir  string
	Agent       AgentKind
	Ticket      tickets.Ticket
}

// IterationWorktree is what PrepareIteration made for one ticket.
type IterationWorktree struct {
	Path            string
	Branch          string
	Label           string
	base            string
	featureWorktree string
	worktreeDir     string
}

// worktreeLock serializes git worktree add/remove per process, as Run's
// worktreeMu does per run.
var worktreeLock sync.Mutex

// PrepareIteration creates the epic's feature worktree and the ticket's
// iteration worktree off the feature tip.
func PrepareIteration(d Deps, o OneIteration) (IterationWorktree, error) {
	wtDir, err := d.WorktreeDir(o.RepoDir)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving worktree directory for %q: %w", o.RepoDir, err)
	}
	featurePath := filepath.Join(wtDir, o.Epic)
	worktreeLock.Lock()
	defer worktreeLock.Unlock()
	if err := d.AddWorktree(o.RepoDir, featurePath, o.Epic, ""); err != nil {
		return IterationWorktree{}, fmt.Errorf("creating feature worktree for branch %q: %w", o.Epic, err)
	}
	branch := iterBranch(o.Epic, o.Ticket.Identifier)
	base, err := d.RevParse(featurePath, o.Epic)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving %s tip: %w", o.Epic, err)
	}
	path := iterationWorktreePath(wtDir, o.Epic, o.Ticket.Identifier)
	if err := d.AddWorktree(o.RepoDir, path, branch, base); err != nil {
		return IterationWorktree{}, fmt.Errorf("creating iteration worktree: %w", err)
	}
	return IterationWorktree{
		Path: path, Branch: branch, Label: iterLabel(o.Epic, o.Ticket.Identifier),
		base: base, featureWorktree: featurePath, worktreeDir: wtDir,
	}, nil
}

// DiscardIteration removes a prepared iteration's worktree and branch, for a
// launch that failed before the agent produced anything.
func DiscardIteration(d Deps, o OneIteration, w IterationWorktree) error {
	return finishCleanup(d, &worktreeLock, o.RepoDir, w.featureWorktree, w.Path, w.Branch, "", true)
}

// FinishIteration runs a finished agent through the shared finish path: park
// on a needs-answer or zero-commit finish, otherwise land the commits onto the
// feature branch, mark the ticket done and clean up. A deferred land (lock
// held) is retried by the caller on its next pass.
func FinishIteration(d Deps, o OneIteration, w IterationWorktree, pane, tab string) error {
	p := iterationParams{
		WorkspaceID:     o.WorkspaceID,
		RepoDir:         o.RepoDir,
		WorktreeDir:     w.worktreeDir,
		FeatureWorktree: w.featureWorktree,
		FeatureBranch:   o.Epic,
		Agent:           o.Agent,
		Skill:           defaultWorkerSkill,
		Ticket:          o.Ticket,
		ScratchDir:      o.ScratchDir,
		WorktreeLock:    &worktreeLock,
		SmartZone:       defaultSmartZone,
		Gate:            NewGate(),
		Sink:            noopEventSink{},
	}
	err := finishIteration(d, p, w.Path, pane, tab, w.base, w.Branch, "")
	var built *builtAwaitingLandError
	if !errors.As(err, &built) {
		return err
	}
	lp := landQueueParams{
		WorkspaceID: p.WorkspaceID, RepoDir: p.RepoDir, WorktreeDir: p.WorktreeDir,
		FeatureWorktree: p.FeatureWorktree, FeatureBranch: p.FeatureBranch, Agent: p.Agent,
		Skill: p.Skill, ScratchDir: p.ScratchDir, WorktreeLock: p.WorktreeLock,
		SmartZone: p.SmartZone, Gate: p.Gate, Sink: p.Sink,
	}
	r := landOne(d, lp, built.job)
	switch {
	case r.err != nil:
		return r.err
	case r.landDeferred:
		return errors.New("land deferred: land lock held")
	}
	return nil
}
