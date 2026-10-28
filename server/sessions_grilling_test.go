package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// startGrillingSession starts a fake session on a map epic (Design) or a plain
// one, with claude's config dir pointed at a temp dir, and returns the
// transcript path of its conversation.
func startGrillingSession(t *testing.T, ticket string) (*servertest.Harness, context.Context, <-chan server.Event, string, string, string) {
	t.Helper()
	return startGrillingSessionIn(t, ticket, t.TempDir())
}

// startGrillingSessionIn is startGrillingSession on a store the caller has
// already seeded.
func startGrillingSessionIn(t *testing.T, ticket, store string, opts ...func(*server.Config)) (*servertest.Harness, context.Context, <-chan server.Event, string, string, string) {
	t.Helper()
	cfg := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	// A caller may seed the map with a design_step first.
	if mapFile := filepath.Join(store, "proj", "map", "ticket.md"); !strings.Contains(string(mustRead(mapFile)), "kind: map") {
		if err := os.WriteFile(mapFile, []byte("---\nkind: map\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts = append([]func(*server.Config){servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)), withAlerts(true, true), servertest.WithEnv("CLAUDE_CONFIG_DIR=" + cfg)}, opts...)
	h := servertest.StartWithStore(t, store, opts...)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: ticket})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, h, id)["conversation_id"].(string)
	return h, ctx, evs, id, conv, filepath.Join(cfg, "projects", "-repo", conv+".jsonl")
}

// mustRead is a file's content, or nil when it does not exist yet.
func mustRead(path string) []byte {
	b, _ := os.ReadFile(path)
	return b
}

// storeWithMapStep is a ticket store whose map epic is a map at step.
func storeWithMapStep(t *testing.T, step string) string {
	t.Helper()
	store := t.TempDir()
	dir := filepath.Join(store, "proj", "map")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ticket.md"), []byte("---\nkind: map\ndesign_step: "+step+"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return store
}

func writeAssistantMessage(t *testing.T, path, text string) {
	t.Helper()
	line, err := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []map[string]any{{"type": "text", "text": text}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(line, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
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
			waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
			report(t, conv, agentfake.Control{State: "idle"})
			got := waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
			if tc.raised {
				got = waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Reason != "" })
				if got.Actionable.Reason != "Grilling: waiting for your answers" {
					t.Fatalf("actionable = %+v, want the grilling reason", got.Actionable)
				}
				clearByInput(t, h, id)
				waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Reason == "" })
				return
			}
			// A later status change orders the check after the detection ran.
			report(t, conv, agentfake.Control{State: "working"})
			got = waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
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
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	line, err := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []map[string]any{{"type": "text", "text": "❓ **Q1** Which store?"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	appendLines(t, path, string(line), turnDurationLine)
	got := waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Reason != "" })
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
			waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
			report(t, conv, agentfake.Control{State: "idle"})
			waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
			if !tc.wantAction {
				report(t, conv, agentfake.Control{State: "working"})
				got := waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
				if got.Actionable.Reason != "" {
					t.Errorf("actionable = %+v, want none", got.Actionable)
				}
				return
			}
			got := waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Reason != "" }).Actionable
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
		"branch":     {step: "branch"},
		"done":       {step: "done"},
		"spec":       {step: "spec", wantAction: true},
		"unreadable": {step: "no-such-step", wantAction: true},
	} {
		t.Run(name, func(t *testing.T) {
			h, ctx, evs, id, conv, path := startGrillingSessionIn(t, "proj:map", storeWithMapStep(t, tc.step))
			writeAssistantMessage(t, path, line)
			report(t, conv, agentfake.Control{State: "working"})
			waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
			report(t, conv, agentfake.Control{State: "idle"})
			got := waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Actionable.Reason != "" }).Actionable
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

func clearByInput(t *testing.T, h *servertest.Harness, id string) {
	t.Helper()
	c, err := nativerunner.DialHolder(filepath.Join(h.SessionsDir, id))
	if err != nil {
		t.Fatal(err)
	}
	att, err := c.Attach()
	if err != nil {
		t.Fatal(err)
	}
	// Closing right after the send can drop the key before the holder reads it, so the
	// attachment lives until the test ends.
	t.Cleanup(func() { att.Close() })
	if err := att.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
		t.Fatal(err)
	}
}
