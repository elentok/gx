package server_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// startDraftSession starts a fake session whose input box already holds a
// draft.
func startDraftSession(t *testing.T, req wire.CreateSessionRequest, draft string) (*servertest.Harness, context.Context, <-chan server.Event, string, string) {
	t.Helper()
	return startFakeSessionWith(t, req, withAlerts(true, true), servertest.WithEnv(agentfake.ProgramDraftEnv+"="+draft, agentfake.ProgramInputEnv+"=1"))
}

func TestDraft_FirstPromptClearsTheDraftAndTypes(t *testing.T) {
	t.Parallel()
	_, _, _, _, conv := startDraftSession(t, wire.CreateSessionRequest{Prompt: "fix the bug"}, "my draft")
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
}

func TestDraft_MultiLineDraftIsCleared(t *testing.T) {
	t.Parallel()
	_, _, _, _, conv := startDraftSession(t, wire.CreateSessionRequest{Prompt: "fix the bug"}, "one\ntwo\nthree\nfour\nfive")
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
}

func TestDraft_ActionClearsTheDraftAndTypes(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startDraftSession(t, wire.CreateSessionRequest{}, "my draft")
	idleWithAction(ctx, t, h, evs, id, conv, "first-step")

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	waitForSubmitted(t, conv, "\"first-step\"\n")
}

func TestDraft_ActionDraftBeforeLaterStepIsCleared(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSessionWith(t, wire.CreateSessionRequest{}, withAlerts(true, true), servertest.WithEnv(agentfake.ProgramInputEnv+"=1"))
	idleWithAction(ctx, t, h, evs, id, conv, "first-step", "second-step")

	if res, err := h.Client.RunSessionAction(ctx, id); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	waitForSubmitted(t, conv, "\"first-step\"\n")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })

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
	// Not a bare "x": the placeholder's "fix" has one, so it matches at once.
	waitForScreen(t, c, "❯ x")
	report(t, conv, agentfake.Control{State: "done"})

	waitForSubmitted(t, conv, "\"first-step\"\n\"second-step\"\n")
}
