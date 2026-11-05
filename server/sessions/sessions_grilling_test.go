package sessions_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// startGrillingSession starts a fake session on a map epic (Design) or a plain
// one, with claude's config dir pointed at a temp dir, and returns the
// transcript path of its conversation.
func startGrillingSession(t *testing.T, ticket string) (*servertest.Harness, context.Context, <-chan wire.Event, string, string, string) {
	t.Helper()
	return startGrillingSessionIn(t, ticket, t.TempDir())
}

func TestGrilling_DetectedOnIdleForDesignSessions(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		ticket, text string
		raised       bool
	}{
		"design with marker":    {"proj:map", "Round one\n\n❓ **Q1** Which store?", true},
		"design without marker": {"proj:map", "All done, nothing to ask.", false},
		"plain with marker":     {"proj:plain", "❓ **Q1** Which store?", false},
	} {
		t.Run(name, func(t *testing.T) {
			h, ctx, evs, id, conv, path := startGrillingSession(t, tc.ticket)
			writeAssistantMessage(t, path, tc.text)
			report(t, conv, agentfake.Control{State: "working"})
			waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
			report(t, conv, agentfake.Control{State: "idle"})
			got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
			if tc.raised {
				got = waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" })
				if got.Actionable.Reason != "Grilling: waiting for your answers" {
					t.Fatalf("actionable = %+v, want the grilling reason", got.Actionable)
				}
				clearByInput(t, h, id)
				waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Actionable.Reason == "" })
				return
			}
			// A later status change orders the check after the detection ran.
			report(t, conv, agentfake.Control{State: "working"})
			got = waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
			if got.Actionable.Reason != "" {
				t.Errorf("actionable = %+v, want none", got.Actionable)
			}
		})
	}
}

// Claude reports idle before it writes the turn's last lines, so the idle
// edge's own read sees the previous message.
func TestGrilling_DetectedWhenTheTurnLandsAfterIdle(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := startGrillingSession(t, "proj:map")
	writeAssistantMessage(t, path, "Reading the code.")
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	line, err := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []map[string]any{{"type": "text", "text": "❓ **Q1** Which store?"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	appendLines(t, path, string(line), turnDurationLine)
	got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" })
	if !got.Actionable.Grilling {
		t.Fatalf("actionable = %+v, want the grilling round", got.Actionable)
	}
}

func TestRunEnd_DetectedOnIdleForDesignSessions(t *testing.T) {
	t.Parallel()
	const grill = "❓ **Q1** Which store?\n"
	for name, tc := range map[string]struct {
		text       string
		reason     string
		kind       string
		target     string
		action     []string
		wantAction bool
	}{
		"marker":        {text: "Done.\n✅ Map: drafted. Next: start a fresh session", reason: "✅ Map: drafted. Next: start a fresh session", action: []string{"/clear", "/gx-design map"}, wantAction: true},
		"grilling wins": {text: grill + "✅ Map: drafted. Next: more", reason: "Grilling: waiting for your answers", wantAction: true},
		"neither":       {text: "All done, nothing to ask."},
	} {
		t.Run(name, func(t *testing.T) {
			h, ctx, evs, id, conv, path := startGrillingSession(t, "proj:map")
			writeAssistantMessage(t, path, tc.text)
			report(t, conv, agentfake.Control{State: "working"})
			waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
			report(t, conv, agentfake.Control{State: "idle"})
			waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
			if !tc.wantAction {
				report(t, conv, agentfake.Control{State: "working"})
				got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
				if got.Actionable.Reason != "" {
					t.Errorf("actionable = %+v, want none", got.Actionable)
				}
				return
			}
			got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" }).Actionable
			if got.Reason != tc.reason || got.Kind != tc.kind || got.Target != tc.target || !slices.Equal(got.Action, tc.action) {
				t.Errorf("actionable = %+v, want reason %q kind %q target %q action %v", got, tc.reason, tc.kind, tc.target, tc.action)
			}
		})
	}
}

func TestRunEnd_NoContinueAtBranchAndDone(t *testing.T) {
	t.Parallel()
	const line = "✅ Map: branch cut. Next: implement"
	for name, tc := range map[string]struct {
		step       string
		wantAction bool
	}{
		// No "done": a first-seen done step makes scanDesignCleanups stop the live session.
		"branch":     {step: "branch"},
		"spec":       {step: "spec", wantAction: true},
		"unreadable": {step: "no-such-step", wantAction: true},
	} {
		t.Run(name, func(t *testing.T) {
			h, ctx, evs, id, conv, path := startGrillingSessionIn(t, "proj:map", storeWithMapStep(t, tc.step))
			writeAssistantMessage(t, path, line)
			report(t, conv, agentfake.Control{State: "working"})
			waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
			report(t, conv, agentfake.Control{State: "idle"})
			got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" }).Actionable
			if got.Reason != line || (len(got.Action) > 0) != tc.wantAction {
				t.Errorf("actionable = %+v, want reason %q, action present = %v", got, line, tc.wantAction)
			}
			if !servertest.Poll(5*time.Second, func() bool { return len(h.Banners.Sent()) > 0 && len(h.Sounds.Played()) > 0 }) {
				t.Fatalf("banners = %v, sounds = %v, want both", h.Banners.Sent(), h.Sounds.Played())
			}
		})
	}
}

func TestSessionChat_NextHasNoActionAtBranch(t *testing.T) {
	t.Parallel()
	store := storeWithMapStep(t, "branch")
	h, ctx, _, id, _, path := startGrillingSessionIn(t, "proj:map", store)
	writeAssistantMessage(t, path, "✅ Map: branch cut. Next: implement")
	got, err := h.Client.SessionChat(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Next == nil || len(got.Next.Action.Action) > 0 {
		t.Errorf("next = %+v, want a run-end line with no action", got.Next)
	}
}

// A design_step change alone, with the transcript untouched, must reach the
// chat's next.
func TestSessionChat_StepChangeAloneRefreshesNext(t *testing.T) {
	t.Parallel()
	store := storeWithMapStep(t, "spec")
	h, ctx, _, id, _, path := startGrillingSessionIn(t, "proj:map", store)
	writeAssistantMessage(t, path, "✅ Map: spec drafted. Next: review")
	got, err := h.Client.SessionChat(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Next == nil || len(got.Next.Action.Action) == 0 {
		t.Fatalf("next = %+v at spec, want Continue", got.Next)
	}
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\ndesign_step: branch\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !servertest.Poll(10*time.Second, func() bool {
		got, err = h.Client.SessionChat(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return got.Next != nil && len(got.Next.Action.Action) == 0
	}) {
		t.Fatalf("next = %+v after the map moved to branch, want no action", got.Next)
	}
}
