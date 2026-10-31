package server

import "github.com/elentok/gx/ralphloop"

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
