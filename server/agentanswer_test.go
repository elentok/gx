package server_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
)

const answerLabel = "epic-a-iter-01"

// herdrFake is a fake that reports itself as the herdr runner.
type herdrFake struct{ *runnerfake.Runner }

func (herdrFake) Name() string { return string(agentrunner.ChoiceHerdr) }

type answerEnv struct {
	h      *servertest.Harness
	fake   *runnerfake.Runner
	ticket string
}

func startAnswerEnv(t *testing.T, wrap func(*runnerfake.Runner) agentrunner.Runner, opts ...func(*server.Config)) answerEnv {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	fake := runnerfake.NewRunner()
	var runner agentrunner.Runner = fake
	if wrap != nil {
		runner = wrap(fake)
	}
	h := servertest.StartWithStore(t, store, append([]func(*server.Config){func(c *server.Config) { c.Runner = runner }}, opts...)...)
	if _, err := fake.Start(agentrunner.StartOptions{Label: answerLabel, Epic: "epic-a", Cwd: t.TempDir(), Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	return answerEnv{h, fake, filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")}
}

func (e answerEnv) permissionEvents(t *testing.T) []tickets.LogEvent {
	t.Helper()
	evs, err := tickets.ReadLog(e.ticket)
	if err != nil {
		t.Fatal(err)
	}
	var out []tickets.LogEvent
	for _, ev := range evs {
		if ev.Type == tickets.EventPausePermission {
			out = append(out, ev)
		}
	}
	return out
}

func (e answerEnv) answer(t *testing.T, req server.QueueRequest) server.QueueResult {
	t.Helper()
	req.Address = "proj:epic-a/01"
	res, err := e.h.Client.AgentAnswerRequest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

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
		{"not blocked", func(e answerEnv) {}, nil, server.ReasonNotBlocked},
		{"request changed", func(e answerEnv) { e.fake.SetPending(answerLabel, bashPending("req-2")) }, nil, server.ReasonRequestChanged},
		{"plan approval", func(e answerEnv) {
			e.fake.SetPending(answerLabel, &agentrunner.Pending{ID: "req-1", Tool: "ExitPlanMode", NotApprovable: true})
		}, nil, server.ReasonNotApprovable},
		{"ask user question", func(e answerEnv) {
			e.fake.SetPending(answerLabel, &agentrunner.Pending{ID: "req-1", Tool: "AskUserQuestion", NotApprovable: true})
		}, nil, server.ReasonNotApprovable},
		{"herdr hosted", func(e answerEnv) { e.fake.SetPending(answerLabel, bashPending("req-1")) },
			func(f *runnerfake.Runner) agentrunner.Runner { return herdrFake{f} }, server.ReasonNotApprovable},
		{"not delivered", func(e answerEnv) {
			e.fake.SetPending(answerLabel, bashPending("req-1"))
			e.fake.SetAnswerErr(answerLabel, agentrunner.ErrNotDelivered)
		}, nil, server.ReasonNotDelivered},
		{"agent gone", func(e answerEnv) {
			e.fake.SetPending(answerLabel, bashPending("req-1"))
			e.fake.SetAnswerErr(answerLabel, agentrunner.SessionDied("claude exited"))
		}, nil, server.ReasonAgentGone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := startAnswerEnv(t, c.wrap)
			c.setup(e)
			res := e.answer(t, server.QueueRequest{Decision: "allow", RequestID: "req-1"})
			if !res.Refused || res.Reason != c.reason {
				t.Fatalf("answer = %+v; want refusal %s", res, c.reason)
			}
			if evs := e.permissionEvents(t); len(evs) != 0 {
				t.Errorf("logged %+v for a refusal", evs)
			}
		})
	}
}

func TestAgentAnswer_AgentGoneWhenNoSessionLives(t *testing.T) {
	t.Parallel()
	e := startAnswerEnv(t, nil)
	if err := e.fake.Stop(mustFind(t, e.fake)); err != nil {
		t.Fatal(err)
	}
	res := e.answer(t, server.QueueRequest{Decision: "allow", RequestID: "req-1"})
	if !res.Refused || res.Reason != server.ReasonAgentGone {
		t.Fatalf("answer = %+v; want refusal %s", res, server.ReasonAgentGone)
	}
}

func mustFind(t *testing.T, r *runnerfake.Runner) agentrunner.Session {
	t.Helper()
	s, ok, err := r.Find(answerLabel)
	if err != nil || !ok {
		t.Fatalf("find = %v, %v", ok, err)
	}
	return s
}

func TestAgentAnswer_DeliveredAllowLogsShownAndIgnoresNote(t *testing.T) {
	t.Parallel()
	e := startAnswerEnv(t, nil)
	e.fake.SetPending(answerLabel, bashPending("req-1"))

	res := e.answer(t, server.QueueRequest{Decision: "allow", RequestID: "req-1", Text: "ignored"})
	if res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	if got := e.fake.Answers(answerLabel); len(got) != 1 || got[0].Text != "" || got[0].RequestID != "req-1" {
		t.Errorf("answers = %+v; want one allow with no note", got)
	}
	evs := e.permissionEvents(t)
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
	e.fake.SetPending(answerLabel, &agentrunner.Pending{
		ID: "req-1", Tool: "Bash", Input: json.RawMessage(strings.Repeat("x", tickets.MaxLogTextChars+1)),
	})

	res := e.answer(t, server.QueueRequest{Decision: "deny", RequestID: "req-1", Text: strings.Repeat("n", tickets.MaxLogTextChars+10)})
	if res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	got := e.fake.Answers(answerLabel)
	if len(got) != 1 || len(got[0].Text) != tickets.MaxLogTextChars {
		t.Fatalf("answers = %d, note length %d; want the note capped at %d", len(got), len(got[0].Text), tickets.MaxLogTextChars)
	}
	evs := e.permissionEvents(t)
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
	e.fake.SetPending(answerLabel, bashPending("req-9"))

	if res := e.answer(t, server.QueueRequest{Decision: "allow"}); res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	if evs := e.permissionEvents(t); len(evs) != 1 || evs[0].Shown == nil || evs[0].Shown.RequestID != "req-9" {
		t.Errorf("events = %+v", evs)
	}
}
