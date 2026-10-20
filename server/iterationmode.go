package server

import (
	"fmt"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// iterationMode is where a ticket runs and how its iteration ends: on the epic's
// feature branch, commitless (a prompt ticket, or anything in the scratch
// project, which has no VCS), or in the feature worktree itself (a merge
// ticket). Decided once per ticket, so launch, finish and resume cannot disagree.
type iterationMode struct {
	commitless bool
	// feature runs in the epic's feature worktree with no iteration branch, and
	// ends like a commitless ticket on a kept directory: only the agent stops.
	feature bool
	// target is where a feature-mode epic lands, resolved at finish so the check
	// sees the trunk current then.
	target func() (string, error)
	// scratchDir is set only for scratch-project tickets.
	scratchDir string
	// ref is the detached worktree's start for a commitless ticket in a real
	// project; launch-only, so resume never needs it.
	ref string
}

func (s *Server) iterationModeFor(addr tickets.Address, t tickets.Ticket) iterationMode {
	typ := schema.TicketType(t.Type)
	if typ == schema.TypeMerge {
		return iterationMode{feature: true, target: func() (string, error) { return s.landTarget(addr) }}
	}
	m := iterationMode{commitless: addr.Project == ScratchProject || addr.Epic == OneOffsEpic || typ == schema.TypePrompt || typ == schema.TypeInvestigate || typ == schema.TypeEvolve || typ == schema.TypeDiagnose}
	if m.commitless && (addr.Project == ScratchProject || s.investigatesWithoutVCS(addr, t)) {
		m.scratchDir = scratchSubdir(s.cfg.TicketStore, addr.Epic)
	}
	return m
}

// investigatesWithoutVCS: an investigate ticket in a vcs: none project has no
// repository to check out, so it runs in a scratch subdir.
func (s *Server) investigatesWithoutVCS(addr tickets.Address, t tickets.Ticket) bool {
	if t.Type != string(schema.TypeInvestigate) {
		return false
	}
	dir, err := s.projectDir(addr.Project)
	if err != nil {
		return false
	}
	pf, _ := config.ReadProjectFile(dir)
	return pf.VCS != nil && *pf.VCS == config.VCSNone
}

func (m iterationMode) prepare(d ralphloop.Deps, o ralphloop.OneIteration) (ralphloop.IterationWorktree, error) {
	switch {
	case m.feature:
		return ralphloop.PrepareFeature(d, o)
	case m.commitless:
		return ralphloop.PrepareCommitless(d, o, m.scratchDir, m.ref)
	}
	return ralphloop.PrepareIteration(d, o)
}

func (m iterationMode) discard(d ralphloop.Deps, o ralphloop.OneIteration, w ralphloop.IterationWorktree) error {
	if m.commitless || m.feature {
		return ralphloop.DiscardCommitless(d, o, w)
	}
	return ralphloop.DiscardIteration(d, o, w)
}

func (m iterationMode) finish(d ralphloop.Deps, o ralphloop.OneIteration, w ralphloop.IterationWorktree, pane string) (ralphloop.FinishOutcome, error) {
	switch {
	case m.feature:
		target, err := m.target()
		if err != nil {
			return ralphloop.FinishOutcome{}, fmt.Errorf("resolving merge target: %w", err)
		}
		return ralphloop.FinishMerge(d, o, w, pane, target)
	case m.commitless:
		return ralphloop.FinishCommitless(d, o, w, pane)
	}
	return ralphloop.FinishIteration(d, o, w, pane)
}

// resume rebuilds the worktree; base is the persisted base of a feature-branch run.
func (m iterationMode) resume(d ralphloop.Deps, o ralphloop.OneIteration, base string) (ralphloop.IterationWorktree, error) {
	switch {
	case m.feature:
		return ralphloop.ResumeFeature(d, o)
	case m.commitless:
		return ralphloop.ResumeCommitless(d, o, m.scratchDir)
	}
	return ralphloop.ResumeIteration(d, o, base)
}
