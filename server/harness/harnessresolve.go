package harness

import (
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/queue"
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

// Inputs are the four levels, highest first. An empty or unknown value at a
// level is skipped.
type Inputs struct {
	Frontmatter string // the ticket's `agent:`
	QueueItem   string // the queue item's agent, applies to every ticket of its root
	EarlierRun  string // harness of the ticket's earlier run; empty when it never ran
	Default     string // the Default harness; empty means claude
}

// ResolvedHarness is the harness a ticket launches under and what decided it.
type ResolvedHarness struct {
	Harness agent.AgentKind
	Source  HarnessSource
}

// DiffersFromEarlierRun reports that the harness is not the one the ticket
// last ran under, so the caller starts the new session from the Handoff.
func (r ResolvedHarness) DiffersFromEarlierRun(earlierRun string) bool {
	return earlierRun != "" && agent.ValidateAgentKind(agent.AgentKind(earlierRun)) == nil &&
		agent.AgentKind(earlierRun) != r.Harness
}

// Resolve decides a ticket's harness: frontmatter > queue item > earlier run >
// default.
func Resolve(in Inputs) ResolvedHarness {
	levels := []struct {
		value  string
		source HarnessSource
	}{
		{in.Frontmatter, HarnessFromFrontmatter},
		{in.QueueItem, HarnessFromQueueItem},
		{in.EarlierRun, HarnessFromEarlierRun},
	}
	for _, l := range levels {
		if l.value != "" && agent.ValidateAgentKind(agent.AgentKind(l.value)) == nil {
			return ResolvedHarness{Harness: agent.AgentKind(l.value), Source: l.source}
		}
	}
	return ResolvedHarness{Harness: DefaultOrClaude(in.Default), Source: HarnessFromDefault}
}

// DefaultOrClaude is name as a harness, claude when it is empty or unknown.
func DefaultOrClaude(name string) agent.AgentKind {
	if name != "" && agent.ValidateAgentKind(agent.AgentKind(name)) == nil {
		return agent.AgentKind(name)
	}
	return agent.AgentClaude
}

// DefaultHarness is the Default harness for project: its agents.default-harness
// over the global one, claude when neither is set.
func (h *Harness) DefaultHarness(project string) agent.AgentKind {
	return DefaultOrClaude(h.projects.AgentsConfig(project).DefaultHarness)
}

// TicketHarness is a ticket's resolved harness and the harness of its earlier
// run ("" when it never ran).
type TicketHarness struct {
	ResolvedHarness
	EarlierRun string
}

// DiffersFromEarlierRun reports that the ticket starts under another harness
// than its earlier run, so it starts from the Handoff.
func (h TicketHarness) DiffersFromEarlierRun() bool {
	return h.ResolvedHarness.DiffersFromEarlierRun(h.EarlierRun)
}

// ResolveTicket gathers the resolver's inputs for the ticket at addr: its
// frontmatter agent, the queue item of its root, its earlier run and the
// Default harness. Every site that picks a harness for a ticket calls it.
func (h *Harness) ResolveTicket(addr tickets.Address, t tickets.Ticket) TicketHarness {
	earlier := h.EarlierRunHarness(addr)
	return TicketHarness{
		ResolvedHarness: Resolve(Inputs{
			Frontmatter: t.Agent,
			QueueItem:   h.queueItemAgent(addr),
			EarlierRun:  earlier,
			Default:     string(h.DefaultHarness(addr.Project)),
		}),
		EarlierRun: earlier,
	}
}

// queueItemAgent is the agent of the queue item for addr's root, "" when none
// names one.
func (h *Harness) queueItemAgent(addr tickets.Address) string {
	for _, item := range h.queue.List() {
		if a, err := tickets.ParseQueuedAddress(item.Address); err == nil && queue.RootOf(a) == queue.RootOf(addr) && item.Agent != "" {
			return item.Agent
		}
	}
	return ""
}

// EarlierRunHarness is the harness of the newest run of addr whose session left
// a transcript (Claude) or a rollout (Codex). A run that died before its first
// prompt left neither, so it does not count and the ticket is never-run.
func (h *Harness) EarlierRunHarness(addr tickets.Address) string {
	dir, err := h.projects.Dir(addr.Project)
	if err != nil {
		return ""
	}
	evs, _, err := ralphloop.ReadEvents(dir, addr.Epic)
	if err != nil {
		return ""
	}
	for _, r := range ralphloop.IterationSessions(evs, addr.ID) {
		if h.sessionRecorded(r) {
			return string(r.Agent)
		}
	}
	return ""
}

func (h *Harness) sessionRecorded(r ralphloop.IterationSession) bool {
	key := string(r.Agent) + "\x00" + r.Cwd + "\x00" + r.Session
	now := time.Now()
	if found, ok := h.recorded.lookup(key, now); ok {
		return found
	}
	found := h.sessionRecordedUncached(r)
	h.recorded.store(key, found, now)
	return found
}

func (h *Harness) sessionRecordedUncached(r ralphloop.IterationSession) bool {
	switch r.Agent {
	case agent.AgentClaude:
		home, err := h.projects.EnvHome()
		if err != nil {
			return false
		}
		found, err := transcript.FindByID(home, r.Session)
		return err == nil && len(found) > 0
	case agent.AgentCodex:
		home, err := h.projects.CodexHome()
		if err != nil {
			return false
		}
		ok, err := codexsession.VerifyIdentityIn(home, r.Cwd, r.Session)
		return err == nil && ok
	}
	return false
}
