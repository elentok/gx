package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder())
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

func TestRunEnd_DetectedOnIdleForDesignSessions(t *testing.T) {
	const grill = "❓ **Q1** Which store?\n"
	for name, tc := range map[string]struct {
		text       string
		reason     string
		kind       string
		target     string
		action     []string
		wantAction bool
	}{
		"next map":       {text: "Done.\n✅ Map: drafted. Next: map — more to split", reason: "✅ Map: drafted. Next: map — more to split", action: []string{"/clear", "/gx-design map"}, wantAction: true},
		"next review":    {text: "✅ Map: drafted. Next: review — check it", reason: "✅ Map: drafted. Next: review — check it", action: []string{"/clear", "/gx-design map"}, wantAction: true},
		"next implement": {text: "✅ Map: drafted. Next: implement — build it", reason: "✅ Map: drafted. Next: implement — build it", kind: "queue", target: "proj:map-impl", wantAction: true},
		"grilling wins":  {text: grill + "✅ Map: drafted. Next: map — more", reason: "Grilling: waiting for your answers", wantAction: true},
		"neither":        {text: "All done, nothing to ask."},
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
	defer att.Close()
	if err := att.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
		t.Fatal(err)
	}
}
