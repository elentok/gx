package server_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

const (
	grillTurn   = "❓ **Q1** Which store?\n"
	runEndTurn  = "Done.\n✅ Map: drafted. Next: start a fresh session"
	runEndLine  = "✅ Map: drafted. Next: start a fresh session"
	plainTurn   = "All done, nothing to ask."
	grillReason = "Grilling: waiting for your answers"
)

// designPromptSession is startPromptSession spread into the arguments the
// helpers here take.
func designPromptSession(t *testing.T) (*servertest.Harness, context.Context, <-chan server.Event, string, string, string) {
	t.Helper()
	f := startPromptSession(t)
	return f.h, f.ctx, f.evs, f.id, f.conv, f.transcript
}

// idleWith ends a turn with text: working, then idle.
func idleWith(ctx context.Context, t *testing.T, h *servertest.Harness, evs <-chan server.Event, id, conv, path, text string) {
	t.Helper()
	writeAssistantMessage(t, path, text)
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
}

// settled returns the Actionable after the detection of the last idle ran: a
// later status change orders the read after it.
func settled(ctx context.Context, t *testing.T, h *servertest.Harness, evs <-chan server.Event, id, conv string) server.SessionActionable {
	t.Helper()
	report(t, conv, agentfake.Control{State: "working"})
	return waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking }).Actionable
}

func TestDerivedActionable_PageAnswerClearsGrilling(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := designPromptSession(t)
	idleWith(ctx, t, h, evs, id, conv, path, grillTurn)
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Grilling })
	res, err := h.Client.PromptSession(ctx, server.SessionPromptRequest{ID: id, Text: "my answer"})
	if err != nil || res.Refused {
		t.Fatalf("prompt = %+v, %v", res, err)
	}
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Reason == "" })
}

func TestDerivedActionable_RunEndAfterGrilling(t *testing.T) {
	t.Parallel()
	for name, answer := range map[string]func(*testing.T, *servertest.Harness, context.Context, string){
		"pane input": func(t *testing.T, h *servertest.Harness, _ context.Context, id string) { clearByInput(t, h, id) },
		"page prompt": func(t *testing.T, h *servertest.Harness, ctx context.Context, id string) {
			if _, err := h.Client.PromptSession(ctx, server.SessionPromptRequest{ID: id, Text: "my answer"}); err != nil {
				t.Fatal(err)
			}
		},
		"no answer": func(*testing.T, *servertest.Harness, context.Context, string) {},
	} {
		t.Run(name, func(t *testing.T) {
			h, ctx, evs, id, conv, path := designPromptSession(t)
			idleWith(ctx, t, h, evs, id, conv, path, grillTurn)
			waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Grilling })
			answer(t, h, ctx, id)
			idleWith(ctx, t, h, evs, id, conv, path, runEndTurn)
			got := waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Reason == runEndLine }).Actionable
			if len(got.Action) != 2 || got.Grilling {
				t.Errorf("actionable = %+v, want Continue with its action", got)
			}
		})
	}
}

func TestDerivedActionable_PlainTurnAfterGrillingClears(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := designPromptSession(t)
	idleWith(ctx, t, h, evs, id, conv, path, grillTurn)
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Grilling })
	idleWith(ctx, t, h, evs, id, conv, path, plainTurn)
	if got := settled(ctx, t, h, evs, id, conv); got.Reason != "" {
		t.Errorf("actionable = %+v, want none", got)
	}
}

func TestDerivedActionable_AgentRaised(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		text   string
		reason string
	}{
		"plain turn keeps it":       {plainTurn, "review ready"},
		"run-end turn replaces it":  {runEndTurn, runEndLine},
		"grilling turn replaces it": {grillTurn, grillReason},
	} {
		t.Run(name, func(t *testing.T) {
			h, ctx, evs, id, conv, path := designPromptSession(t)
			if res, err := h.Client.MarkSessionActionable(ctx, server.MarkActionableRequest{ID: id, Reason: "review ready"}); err != nil || res.Refused {
				t.Fatalf("mark = %+v, %v", res, err)
			}
			idleWith(ctx, t, h, evs, id, conv, path, tc.text)
			if got := settled(ctx, t, h, evs, id, conv); got.Reason != tc.reason {
				t.Errorf("actionable = %+v, want reason %q", got, tc.reason)
			}
		})
	}
}
