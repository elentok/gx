package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// actionWatchSettle holds a status long enough for the Action's own watch,
// which samples on a ticker, to see it.
const actionWatchSettle = 200 * time.Millisecond

// idleWithAction brings the session Idle and raises an Actionable carrying
// action.
func idleWithAction(ctx context.Context, t *testing.T, h *servertest.Harness, evs <-chan server.Event, id, conv string, action ...string) {
	t.Helper()
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	res, err := h.Client.MarkSessionActionable(ctx, server.MarkActionableRequest{ID: id, Reason: "ready", Action: action})
	if err != nil || res.Refused {
		t.Fatalf("mark actionable = %+v, %v", res, err)
	}
}

func TestRunAction_SendsEachPromptAfterWorkingThenIdle(t *testing.T) {
	h, ctx, evs, id, conv := startFakeSession(t)
	idleWithAction(ctx, t, h, evs, id, conv, "first-step", "second-step")

	res, err := h.Client.RunSessionAction(ctx, id)
	if err != nil || res.Refused || res.Session == nil {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	if res.Session.Actionable.Reason != "" {
		t.Errorf("actionable = %+v after a run, want none", res.Session.Actionable)
	}
	if _, ok := readMeta(t, h, id)["actionable"]; ok {
		t.Error("actionable still saved after a run")
	}

	c := waitForHolder(t, filepath.Join(h.SessionsDir, id))
	waitForScreen(t, c, "first-step")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	time.Sleep(actionWatchSettle)
	if screenHas(t, c, "second-step") {
		t.Fatal("second prompt sent while the first was working")
	}
	report(t, conv, agentfake.Control{State: "done"})
	waitForScreen(t, c, "second-step")
}

func TestRunAction_StepThatNeverWorksStopsTheRest(t *testing.T) {
	const timeout = 200 * time.Millisecond
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{}, func(c *server.Config) { c.ActionStepStartTimeout = timeout })
	idleWithAction(ctx, t, h, evs, id, conv, "first-step", "second-step")

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	c := waitForHolder(t, filepath.Join(h.SessionsDir, id))
	waitForScreen(t, c, "first-step")
	time.Sleep(3 * timeout)
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	report(t, conv, agentfake.Control{State: "done"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	time.Sleep(3 * timeout)
	if screenHas(t, c, "second-step") {
		t.Error("second prompt sent after the first never went working")
	}
}

func TestRunAction_SlashCommandThatNeverWorksStillSendsTheRest(t *testing.T) {
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{}, func(c *server.Config) { c.ActionStepStartTimeout = 200 * time.Millisecond })
	idleWithAction(ctx, t, h, evs, id, conv, "/clear", "/gx-design X")

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	c := waitForHolder(t, filepath.Join(h.SessionsDir, id))
	waitForScreen(t, c, "/clear")
	waitForScreen(t, c, "/gx-design X")
}

// startInputSession is startFakeSessionWith on a fake program with an input
// box, so a slash step has a real input to check.
func startInputSession(t *testing.T, settle time.Duration) (*servertest.Harness, context.Context, <-chan server.Event, string, string) {
	t.Helper()
	t.Setenv(agentfake.ProgramInputEnv, "1")
	return startFakeSessionWith(t, server.CreateSessionRequest{}, func(c *server.Config) {
		c.ActionStepStartTimeout = time.Minute
		c.ActionSlashSettle = settle
	})
}

func submitted(t *testing.T, conv string) string {
	t.Helper()
	data, _ := os.ReadFile(agentfake.ProgramSubmittedPath(conv))
	return string(data)
}

func waitForSubmitted(t *testing.T, conv, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for submitted(t, conv) != want {
		if time.Now().After(deadline) {
			t.Fatalf("submitted = %q, want %q", submitted(t, conv), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunAction_SlashStepEndsOnNewConversationWithoutWaiting(t *testing.T) {
	// A settle window far past the test's own deadline: only the new
	// conversation id can end the first step in time.
	h, ctx, evs, id, conv := startInputSession(t, time.Minute)
	idleWithAction(ctx, t, h, evs, id, conv, agentfake.ProgramClearCommand, "next-step")

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	waitForSubmitted(t, conv, "\"/clear\"\n\"next-step\"\n")
}

func TestRunAction_LocalSlashStepEndsOnSettleWithoutAnotherEnter(t *testing.T) {
	const settle = 200 * time.Millisecond
	h, ctx, evs, id, conv := startInputSession(t, settle)
	idleWithAction(ctx, t, h, evs, id, conv, "/local", "next-step")

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	waitForSubmitted(t, conv, "\"/local\"\n\"next-step\"\n")
}

func TestRunAction_SwallowedEnterOnSlashStepGetsOneEnter(t *testing.T) {
	const settle = 200 * time.Millisecond
	h, ctx, evs, id, conv := startInputSession(t, settle)
	idleWithAction(ctx, t, h, evs, id, conv, "/local")
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
		t.Fatal(err)
	}

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	waitForSubmitted(t, conv, "\"/local\"\n")
	time.Sleep(3 * settle)
	if got := submitted(t, conv); got != "\"/local\"\n" {
		t.Errorf("submitted = %q, want exactly one submission", got)
	}
}

func TestRunAction_RealPromptStepStillNeedsATurn(t *testing.T) {
	const settle = 100 * time.Millisecond
	h, ctx, evs, id, conv := startInputSession(t, settle)
	idleWithAction(ctx, t, h, evs, id, conv, "real-prompt", "next-step")

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	waitForSubmitted(t, conv, "\"real-prompt\"\n")
	time.Sleep(5 * settle)
	if got := submitted(t, conv); got != "\"real-prompt\"\n" {
		t.Errorf("submitted = %q, want the next step held until a turn", got)
	}
	report(t, conv, agentfake.Control{State: "working"})
	time.Sleep(actionWatchSettle)
	report(t, conv, agentfake.Control{State: "done"})
	waitForSubmitted(t, conv, "\"real-prompt\"\n\"next-step\"\n")
}

func TestRunAction_BlockedStepStopsTheRest(t *testing.T) {
	const timeout = 200 * time.Millisecond
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{}, func(c *server.Config) { c.ActionStepStartTimeout = timeout })
	idleWithAction(ctx, t, h, evs, id, conv, "first-step", "second-step")

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	c := waitForHolder(t, filepath.Join(h.SessionsDir, id))
	waitForScreen(t, c, "first-step")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	report(t, conv, agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionBlocked })
	time.Sleep(actionWatchSettle)
	report(t, conv, agentfake.Control{State: "done"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	time.Sleep(3 * timeout)
	if screenHas(t, c, "second-step") {
		t.Error("second prompt sent after the first went blocked")
	}
}

func TestRunAction_RefusedWhileWorkingKeepsActionable(t *testing.T) {
	h, ctx, evs, id, conv := startFakeSession(t)
	idleWithAction(ctx, t, h, evs, id, conv, "/review")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })

	res, err := h.Client.RunSessionAction(ctx, id)
	if err != nil || !res.Refused || res.Reason != server.ReasonSessionNotIdle {
		t.Errorf("run while working = %+v, %v; want refused %s", res, err, server.ReasonSessionNotIdle)
	}
	if saved, _ := readMeta(t, h, id)["actionable"].(map[string]any); saved["reason"] != "ready" {
		t.Errorf("saved actionable = %v, want it kept", saved)
	}
}

func TestRunAction_RefusedWithNoAction(t *testing.T) {
	h, ctx, evs, id, conv := startFakeSession(t)
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || !res.Refused || res.Reason != server.ReasonSessionNoAction {
		t.Errorf("run with no actionable = %+v, %v; want refused %s", res, err, server.ReasonSessionNoAction)
	}

	if res, err := h.Client.MarkSessionActionable(ctx, server.MarkActionableRequest{ID: id, Reason: "ready"}); err != nil || res.Refused {
		t.Fatalf("mark actionable = %+v, %v", res, err)
	}
	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || !res.Refused || res.Reason != server.ReasonSessionNoAction {
		t.Errorf("run with no action = %+v, %v; want refused %s", res, err, server.ReasonSessionNoAction)
	}
	if saved, _ := readMeta(t, h, id)["actionable"].(map[string]any); saved["reason"] != "ready" {
		t.Errorf("saved actionable = %v, want it kept", saved)
	}
}
