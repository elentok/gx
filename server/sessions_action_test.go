package server_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

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
