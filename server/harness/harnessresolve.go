package harness

import (
	"os"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/runlog"
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
	return h.Labeler().resolveTicket(addr, t)
}

// Labeler resolves many tickets in one pass, reading each project's dir and
// agents config and each epic's run-log once instead of once per ticket. It
// is a snapshot of those inputs: build one per request, never keep it, and
// use it from one goroutine.
type Labeler struct {
	h        *Harness
	dirs     map[string]projectDir
	defaults map[string]agent.AgentKind
	events   map[string][]runlog.Event
}

type projectDir struct {
	dir string
	err error
}

// Labeler starts a fresh per-request resolution pass.
func (h *Harness) Labeler() *Labeler {
	return &Labeler{h: h, dirs: map[string]projectDir{}, defaults: map[string]agent.AgentKind{}, events: map[string][]runlog.Event{}}
}

func (l *Labeler) resolveTicket(addr tickets.Address, t tickets.Ticket) TicketHarness {
	earlier := l.earlierRunHarness(addr)
	return TicketHarness{
		ResolvedHarness: Resolve(Inputs{
			Frontmatter: t.Agent,
			QueueItem:   l.h.queueItemAgent(addr),
			EarlierRun:  earlier,
			Default:     string(l.defaultHarness(addr.Project)),
		}),
		EarlierRun: earlier,
	}
}

func (l *Labeler) defaultHarness(project string) agent.AgentKind {
	if d, ok := l.defaults[project]; ok {
		return d
	}
	d := l.h.DefaultHarness(project)
	l.defaults[project] = d
	return d
}

func (l *Labeler) projectDir(project string) (string, error) {
	if d, ok := l.dirs[project]; ok {
		return d.dir, d.err
	}
	dir, err := l.h.projects.Dir(project)
	l.dirs[project] = projectDir{dir, err}
	return dir, err
}

// runEvents is the epic's run-log events, nil when unreadable.
func (l *Labeler) runEvents(addr tickets.Address) []runlog.Event {
	key := addr.Project + "\x00" + addr.Epic
	if evs, ok := l.events[key]; ok {
		return evs
	}
	var evs []runlog.Event
	if dir, err := l.projectDir(addr.Project); err == nil {
		if read, _, err := runlog.ReadEvents(dir, addr.Epic); err == nil {
			evs = read
		}
	}
	l.events[key] = evs
	return evs
}

func (l *Labeler) earlierRunHarness(addr tickets.Address) string {
	for _, r := range runlog.IterationSessions(l.runEvents(addr), addr.ID) {
		if l.h.sessionRecorded(r) {
			return string(r.Agent)
		}
	}
	return ""
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
	return h.Labeler().earlierRunHarness(addr)
}

func (h *Harness) sessionRecorded(r runlog.IterationSession) bool {
	key := string(r.Agent) + "\x00" + r.Cwd + "\x00" + r.Session
	now := time.Now()
	if found, ok := h.recorded.lookup(key, now); ok {
		return found
	}
	found := h.sessionRecordedUncached(r)
	h.recorded.store(key, found, now)
	return found
}

func (h *Harness) sessionRecordedUncached(r runlog.IterationSession) bool {
	switch r.Agent {
	case agent.AgentClaude:
		home, err := h.projects.EnvHome()
		if err != nil {
			return false
		}
		// The usual case sits at the cwd's own slug; the glob opens every
		// project dir, so it only runs when that stat misses.
		if r.Cwd != "" {
			if _, err := os.Stat(transcript.PathIn(home, r.Cwd, r.Session)); err == nil {
				return true
			}
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
