package ticketops_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
)

// herdrFake is a fake that reports itself as the herdr runner.
type herdrFake struct{ *runnerfake.Runner }

func (herdrFake) Name() string { return string(agentrunner.ChoiceHerdr) }

func bashPending(id string) *agentrunner.Pending {
	return &agentrunner.Pending{ID: id, Tool: "Bash", Input: json.RawMessage(`{"command":"ls"}`), Description: "list files"}
}

func TestAgentAnswer_RequestIDRefusalsLogNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		setup  func(e answerEnv)
		wrap   func(*runnerfake.Runner) agentrunner.Runner
		reason string
	}{
		{"not blocked", func(e answerEnv) {}, nil, wire.ReasonNotBlocked},
		{"request changed", func(e answerEnv) { e.Fake.SetPending(answerLabel, bashPending("req-2")) }, nil, wire.ReasonRequestChanged},
		{"plan approval", func(e answerEnv) {
			e.Fake.SetPending(answerLabel, &agentrunner.Pending{ID: "req-1", Tool: "ExitPlanMode", NotApprovable: true})
		}, nil, wire.ReasonNotApprovable},
		{"ask user question", func(e answerEnv) {
			e.Fake.SetPending(answerLabel, &agentrunner.Pending{ID: "req-1", Tool: "AskUserQuestion", NotApprovable: true})
		}, nil, wire.ReasonNotApprovable},
		{"herdr hosted", func(e answerEnv) { e.Fake.SetPending(answerLabel, bashPending("req-1")) },
			func(f *runnerfake.Runner) agentrunner.Runner { return herdrFake{f} }, wire.ReasonNotApprovable},
		{"not delivered", func(e answerEnv) {
			e.Fake.SetPending(answerLabel, bashPending("req-1"))
			e.Fake.SetAnswerErr(answerLabel, agentrunner.ErrNotDelivered)
		}, nil, wire.ReasonNotDelivered},
		{"agent gone", func(e answerEnv) {
			e.Fake.SetPending(answerLabel, bashPending("req-1"))
			e.Fake.SetAnswerErr(answerLabel, agentrunner.SessionDied("claude exited"))
		}, nil, wire.ReasonAgentGone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := startAnswerEnv(t, c.wrap)
			c.setup(e)
			res := e.Answer(t, wire.QueueRequest{Decision: "allow", RequestID: "req-1"})
			if !res.Refused || res.Reason != c.reason {
				t.Fatalf("answer = %+v; want refusal %s", res, c.reason)
			}
			if evs := e.PermissionEvents(t); len(evs) != 0 {
				t.Errorf("logged %+v for a refusal", evs)
			}
		})
	}
}

func TestAgentAnswer_AgentGoneWhenNoSessionLives(t *testing.T) {
	t.Parallel()
	e := startAnswerEnv(t, nil)
	if err := e.Fake.Stop(mustFind(t, e.Fake)); err != nil {
		t.Fatal(err)
	}
	res := e.Answer(t, wire.QueueRequest{Decision: "allow", RequestID: "req-1"})
	if !res.Refused || res.Reason != wire.ReasonAgentGone {
		t.Fatalf("answer = %+v; want refusal %s", res, wire.ReasonAgentGone)
	}
}

func TestAgentAnswer_DeliveredAllowLogsShownAndIgnoresNote(t *testing.T) {
	t.Parallel()
	e := startAnswerEnv(t, nil)
	e.Fake.SetPending(answerLabel, bashPending("req-1"))

	res := e.Answer(t, wire.QueueRequest{Decision: "allow", RequestID: "req-1", Text: "ignored"})
	if res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	if got := e.Fake.Answers(answerLabel); len(got) != 1 || got[0].Text != "" || got[0].RequestID != "req-1" {
		t.Errorf("answers = %+v; want one allow with no note", got)
	}
	evs := e.PermissionEvents(t)
	if len(evs) != 1 {
		t.Fatalf("events = %+v; want one pause.permission", evs)
	}
	ev := evs[0]
	if ev.Decision != "allow" || ev.Source != "cli" || ev.Note != "" {
		t.Errorf("event = %+v", ev)
	}
	if s := ev.Shown; s == nil || s.RequestID != "req-1" || s.Tool != "Bash" || s.Description != "list files" || s.Truncated {
		t.Errorf("shown = %+v", ev.Shown)
	}
}

func TestAgentAnswer_DenyNoteIsSentAndCapped(t *testing.T) {
	t.Parallel()
	e := startAnswerEnv(t, nil)
	e.Fake.SetPending(answerLabel, &agentrunner.Pending{
		ID: "req-1", Tool: "Bash", Input: json.RawMessage(strings.Repeat("x", tickets.MaxLogTextChars+1)),
	})

	res := e.Answer(t, wire.QueueRequest{Decision: "deny", RequestID: "req-1", Text: strings.Repeat("n", tickets.MaxLogTextChars+10)})
	if res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	got := e.Fake.Answers(answerLabel)
	if len(got) != 1 || len(got[0].Text) != tickets.MaxLogTextChars {
		t.Fatalf("answers = %d, note length %d; want the note capped at %d", len(got), len(got[0].Text), tickets.MaxLogTextChars)
	}
	evs := e.PermissionEvents(t)
	if len(evs) != 1 || evs[0].Decision != "deny" || len(evs[0].Note) != tickets.MaxLogTextChars {
		t.Fatalf("events = %+v", evs)
	}
	if s := evs[0].Shown; s == nil || !s.Truncated || len(s.Input) != tickets.MaxLogTextChars {
		t.Errorf("shown = %+v; want truncated input", evs[0].Shown)
	}
}

func TestAgentAnswer_WithoutRequestIDAnswersWhateverIsOpen(t *testing.T) {
	t.Parallel()
	e := startAnswerEnv(t, nil)
	e.Fake.SetPending(answerLabel, bashPending("req-9"))

	if res := e.Answer(t, wire.QueueRequest{Decision: "allow"}); res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	if evs := e.PermissionEvents(t); len(evs) != 1 || evs[0].Shown == nil || evs[0].Shown.RequestID != "req-9" {
		t.Errorf("events = %+v", evs)
	}
}
