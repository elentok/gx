package server

import (
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// iterationMode is where a ticket runs and how its iteration ends: on the epic's
// feature branch, or commitless (a prompt ticket, or anything in the scratch
// project, which has no VCS). Decided once per ticket, so launch, finish and
// resume cannot disagree.
type iterationMode struct {
	commitless bool
	// scratchDir is set only for scratch-project tickets.
	scratchDir string
	// ref is the detached worktree's start for a commitless ticket in a real
	// project; launch-only, so resume never needs it.
	ref string
}

func (s *Server) iterationModeFor(addr tickets.Address, t tickets.Ticket) iterationMode {
	m := iterationMode{commitless: addr.Project == ScratchProject || schema.TicketType(t.Type) == schema.TypePrompt}
	if m.commitless && addr.Project == ScratchProject {
		m.scratchDir = scratchSubdir(s.cfg.TicketStore, addr.Epic)
	}
	return m
}

func (m iterationMode) prepare(d ralphloop.Deps, o ralphloop.OneIteration) (ralphloop.IterationWorktree, error) {
	if m.commitless {
		return ralphloop.PrepareCommitless(d, o, m.scratchDir, m.ref)
	}
	return ralphloop.PrepareIteration(d, o)
}

func (m iterationMode) discard(d ralphloop.Deps, o ralphloop.OneIteration, w ralphloop.IterationWorktree) error {
	if m.commitless {
		return ralphloop.DiscardCommitless(d, o, w)
	}
	return ralphloop.DiscardIteration(d, o, w)
}

func (m iterationMode) finish(d ralphloop.Deps, o ralphloop.OneIteration, w ralphloop.IterationWorktree, pane, tab string) (ralphloop.FinishOutcome, error) {
	if m.commitless {
		return ralphloop.FinishCommitless(d, o, w, pane, tab)
	}
	return ralphloop.FinishIteration(d, o, w, pane, tab)
}

// resume rebuilds the worktree; base is the persisted base of a feature-branch run.
func (m iterationMode) resume(d ralphloop.Deps, o ralphloop.OneIteration, base string) (ralphloop.IterationWorktree, error) {
	if m.commitless {
		return ralphloop.ResumeCommitless(d, o, m.scratchDir)
	}
	return ralphloop.ResumeIteration(d, o, base)
}
