package server

import (
	"cmp"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// HarnessSource is the level of the resolution order that decided a harness.
type HarnessSource string

const (
	HarnessFromFrontmatter HarnessSource = "frontmatter"
	HarnessFromQueueItem   HarnessSource = "queue-item"
	HarnessFromEarlierRun  HarnessSource = "earlier-run"
	HarnessFromDefault     HarnessSource = "default"
)

// harnessInputs are the four levels, highest first. An empty or unknown value
// at a level is skipped.
type harnessInputs struct {
	Frontmatter string // the ticket's `agent:`
	QueueItem   string // the queue item's agent, applies to every ticket of its root
	EarlierRun  string // harness of the ticket's earlier run; empty when it never ran
	Default     string // the Default harness; empty means claude
}

// ResolvedHarness is the harness a ticket launches under and what decided it.
type ResolvedHarness struct {
	Harness ralphloop.AgentKind
	Source  HarnessSource
}

// DiffersFromEarlierRun reports that the harness is not the one the ticket
// last ran under, so the caller starts the new session from the Handoff.
func (r ResolvedHarness) DiffersFromEarlierRun(earlierRun string) bool {
	return earlierRun != "" && ralphloop.ValidateAgentKind(ralphloop.AgentKind(earlierRun)) == nil &&
		ralphloop.AgentKind(earlierRun) != r.Harness
}

// resolveHarness decides a ticket's harness: frontmatter > queue item >
// earlier run > default.
func resolveHarness(in harnessInputs) ResolvedHarness {
	levels := []struct {
		value  string
		source HarnessSource
	}{
		{in.Frontmatter, HarnessFromFrontmatter},
		{in.QueueItem, HarnessFromQueueItem},
		{in.EarlierRun, HarnessFromEarlierRun},
	}
	for _, l := range levels {
		if l.value != "" && ralphloop.ValidateAgentKind(ralphloop.AgentKind(l.value)) == nil {
			return ResolvedHarness{Harness: ralphloop.AgentKind(l.value), Source: l.source}
		}
	}
	return ResolvedHarness{Harness: defaultOrClaude(in.Default), Source: HarnessFromDefault}
}

func defaultOrClaude(name string) ralphloop.AgentKind {
	if name != "" && ralphloop.ValidateAgentKind(ralphloop.AgentKind(name)) == nil {
		return ralphloop.AgentKind(name)
	}
	return ralphloop.AgentClaude
}

// defaultHarness is the Default harness for project: its agents.default-harness
// over the global one, claude when neither is set.
func (s *Server) defaultHarness(project string) ralphloop.AgentKind {
	return defaultOrClaude(s.agentsConfig(project).DefaultHarness)
}

// launchHarness is the harness a new interactive session in project starts on
// when its request names none. Unlike defaultHarness it keeps an unregistered
// name, so createSession refuses it instead of silently running Claude.
func (s *Server) launchHarness(project string) string {
	return cmp.Or(s.agentsConfig(project).DefaultHarness, string(ralphloop.AgentClaude))
}

// ticketHarness is a ticket's resolved harness and the harness of its earlier
// run ("" when it never ran).
type ticketHarness struct {
	ResolvedHarness
	EarlierRun string
}

// DiffersFromEarlierRun reports that the ticket starts under another harness
// than its earlier run, so it starts from the Handoff.
func (h ticketHarness) DiffersFromEarlierRun() bool {
	return h.ResolvedHarness.DiffersFromEarlierRun(h.EarlierRun)
}

// resolveTicket gathers the resolver's inputs for the ticket at addr: its
// frontmatter agent, the queue item of its root, its earlier run and the
// Default harness. Every site that picks a harness for a ticket calls it.
func (s *Server) resolveTicket(addr tickets.Address, t tickets.Ticket) ticketHarness {
	earlier := s.earlierRunHarness(addr)
	return ticketHarness{
		ResolvedHarness: resolveHarness(harnessInputs{
			Frontmatter: t.Agent,
			QueueItem:   s.queueItemAgent(addr),
			EarlierRun:  earlier,
			Default:     string(s.defaultHarness(addr.Project)),
		}),
		EarlierRun: earlier,
	}
}

// queueItemAgent is the agent of the queue item for addr's root, "" when none
// names one.
func (s *Server) queueItemAgent(addr tickets.Address) string {
	for _, item := range s.queued.list() {
		if a, err := parseQueuedAddress(item.Address); err == nil && rootOf(a) == rootOf(addr) && item.Agent != "" {
			return item.Agent
		}
	}
	return ""
}

// earlierRunHarness is the harness of the newest run of addr whose session left
// a transcript (Claude) or a rollout (Codex). A run that died before its first
// prompt left neither, so it does not count and the ticket is never-run.
func (s *Server) earlierRunHarness(addr tickets.Address) string {
	dir, err := s.projectDir(addr.Project)
	if err != nil {
		return ""
	}
	evs, _, err := ralphloop.ReadEvents(dir, addr.Epic)
	if err != nil {
		return ""
	}
	for _, r := range ralphloop.IterationSessions(evs, addr.ID) {
		if s.sessionRecorded(r) {
			return string(r.Agent)
		}
	}
	return ""
}

func (s *Server) sessionRecorded(r ralphloop.IterationSession) bool {
	switch r.Agent {
	case ralphloop.AgentClaude:
		home, err := s.envHome()
		if err != nil {
			return false
		}
		found, err := transcript.FindByID(home, r.Session)
		return err == nil && len(found) > 0
	case ralphloop.AgentCodex:
		home, err := s.codexHome()
		if err != nil {
			return false
		}
		ok, err := codexsession.VerifyIdentityIn(home, r.Cwd, r.Session)
		return err == nil && ok
	}
	return false
}
