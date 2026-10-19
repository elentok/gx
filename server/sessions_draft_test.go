package server_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/testutil/agentfake"
)

const draftReason = "input holds a draft"

// startDraftSession starts a fake session whose input box already holds a
// draft.
func startDraftSession(t *testing.T, req server.CreateSessionRequest) (*servertest.Harness, context.Context, <-chan server.Event, string, string) {
	t.Helper()
	t.Setenv(agentfake.ProgramDraftEnv, "my draft")
	t.Setenv(agentfake.ProgramInputEnv, "1")
	return startFakeSessionWith(t, req, withAlerts(true, true))
}

func gxRaised(s server.SessionInfo) bool {
	return s.Actionable.Reason == draftReason && s.Actionable.GxRaised
}

func TestDraft_FirstPromptIsNotTypedAndRaisesActionable(t *testing.T) {
	h, ctx, evs, id, conv := startDraftSession(t, server.CreateSessionRequest{Prompt: "fix the bug"})

	report(t, conv, agentfake.Control{State: "idle"})
	got := waitForSession(ctx, t, h, evs, id, gxRaised)
	if want := []string{"fix the bug"}; !slices.Equal(got.Actionable.Action, want) {
		t.Errorf("action = %v, want %v", got.Actionable.Action, want)
	}
	if s := submitted(t, conv); s != "" {
		t.Errorf("submitted = %q, want nothing typed over the draft", s)
	}
	if saved, _ := readMeta(t, h, id)["actionable"].(map[string]any); saved["gx_raised"] != true {
		t.Errorf("saved actionable = %v, want gx_raised", saved)
	}
	if _, snds := waitAlerts(h, 1); len(snds) == 0 || snds[len(snds)-1] != sound.Attention {
		t.Errorf("sounds = %v, want attention", snds)
	}
}

func TestDraft_GxRaisedSurvivesInputAndAgentRaisedDoesNot(t *testing.T) {
	h, ctx, evs, id, conv := startDraftSession(t, server.CreateSessionRequest{Prompt: "fix the bug"})
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, gxRaised)

	c := waitForHolder(t, filepath.Join(h.SessionsDir, id))
	att, err := c.Attach()
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()
	if err := att.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
		t.Fatal(err)
	}
	// A later status change orders the check after the holder saw the input.
	report(t, conv, agentfake.Control{State: "working"})
	if got := waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking }); !gxRaised(got) {
		t.Errorf("actionable = %+v after input, want the gx-raised one kept", got.Actionable)
	}

	if res, err := h.Client.MarkSessionActionable(ctx, server.MarkActionableRequest{ID: id, Reason: "agent says"}); err != nil || res.Refused || res.Session.Actionable.GxRaised {
		t.Fatalf("mark actionable = %+v, %v", res, err)
	}
	if err := att.SendKey(screen.Key{Code: 'y', Text: "y"}); err != nil {
		t.Fatal(err)
	}
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Reason == "" })
}

func TestDraft_RunWithDraftStillThereRaisesTheSameActionable(t *testing.T) {
	h, ctx, evs, id, conv := startDraftSession(t, server.CreateSessionRequest{Prompt: "fix the bug"})
	report(t, conv, agentfake.Control{State: "idle"})
	first := waitForSession(ctx, t, h, evs, id, gxRaised)

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	got := waitForSession(ctx, t, h, evs, id, gxRaised)
	if !slices.Equal(got.Actionable.Action, first.Actionable.Action) {
		t.Errorf("action = %v, want %v again", got.Actionable.Action, first.Actionable.Action)
	}
	if s := submitted(t, conv); s != "" {
		t.Errorf("submitted = %q, want nothing typed over the draft", s)
	}
}

func TestDraft_ActionDraftBeforeStepOneKeepsTheWholeAction(t *testing.T) {
	h, ctx, evs, id, conv := startDraftSession(t, server.CreateSessionRequest{})
	idleWithAction(ctx, t, h, evs, id, conv, "first-step", "second-step")

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	got := waitForSession(ctx, t, h, evs, id, gxRaised)
	if want := []string{"first-step", "second-step"}; !slices.Equal(got.Actionable.Action, want) {
		t.Errorf("action = %v, want %v", got.Actionable.Action, want)
	}
}

func TestDraft_ActionDraftBeforeLaterStepKeepsOnlyTheUnsentSteps(t *testing.T) {
	t.Setenv(agentfake.ProgramInputEnv, "1")
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{}, withAlerts(true, true))
	idleWithAction(ctx, t, h, evs, id, conv, "first-step", "second-step", "third-step")

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	waitForSubmitted(t, conv, "\"first-step\"\n")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })

	// The person starts a draft while step one runs.
	c := waitForHolder(t, filepath.Join(h.SessionsDir, id))
	att, err := c.Attach()
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()
	if err := att.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, c, "x")
	report(t, conv, agentfake.Control{State: "done"})

	got := waitForSession(ctx, t, h, evs, id, gxRaised)
	if want := []string{"second-step", "third-step"}; !slices.Equal(got.Actionable.Action, want) {
		t.Errorf("action = %v, want %v", got.Actionable.Action, want)
	}
	time.Sleep(200 * time.Millisecond)
	if s := submitted(t, conv); s != "\"first-step\"\n" {
		t.Errorf("submitted = %q, want only the step sent before the draft", s)
	}
}

func TestDraft_ExitClearsActionable(t *testing.T) {
	h, ctx, evs, id, conv := startDraftSession(t, server.CreateSessionRequest{Prompt: "fix the bug"})
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, gxRaised)

	if res, err := h.Client.StopSession(ctx, id); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	got := waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
	if got.Actionable.Reason != "" {
		t.Errorf("actionable = %+v after exit, want none", got.Actionable)
	}
}
