package ralphloop

import (
	"testing"

	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/runlog"
)

func TestLastIterationSession_ReturnsMostRecentMatchingTicket(t *testing.T) {
	t.Parallel()
	events := []runlog.Event{
		{Type: string(eventsc.IterationStarted), Ticket: "01", Agent: agent.AgentClaude, AgentSession: "sess-1a", Cwd: "/cwd-1a"},
		{Type: string(eventsc.IterationFinished), Ticket: "01"},
		{Type: string(eventsc.Paused), Ticket: "01"},
		{Type: string(eventsc.IterationStarted), Ticket: "01", Agent: agent.AgentClaude, AgentSession: "sess-1b", Cwd: "/cwd-1b"},
		{Type: string(eventsc.IterationStarted), Ticket: "02", Agent: agent.AgentClaude, AgentSession: "sess-2", Cwd: "/cwd-2"},
	}

	session, cwd, agentKind, ok := lastIterationSession(events, "01")
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if session != "sess-1b" || cwd != "/cwd-1b" || agentKind != agent.AgentClaude {
		t.Errorf("got (session=%q, cwd=%q, agent=%q), want the most recent iteration-started for ticket 1 (sess-1b/cwd-1b)", session, cwd, agentKind)
	}
}

func TestLastIterationSession_DefaultsAgentForHistoricalLogs(t *testing.T) {
	t.Parallel()
	events := []runlog.Event{
		{Type: string(eventsc.IterationStarted), Ticket: "01", AgentSession: "sess-1"},
	}
	_, _, agentKind, ok := lastIterationSession(events, "01")
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if agentKind != agent.AgentClaude {
		t.Errorf("agent = %q, want agent.AgentClaude default for an event with no recorded Agent", agentKind)
	}
}

func TestLastIterationSession_NoMatch_OkFalse(t *testing.T) {
	t.Parallel()
	events := []runlog.Event{
		{Type: string(eventsc.IterationStarted), Ticket: "02", AgentSession: "sess-2"},
		{Type: string(eventsc.IterationStarted), Ticket: "01", AgentSession: ""},
	}
	_, _, _, ok := lastIterationSession(events, "01")
	if ok {
		t.Error("ok = true, want false when no matching event has a recorded session")
	}
}
