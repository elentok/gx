package servertest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// RunEndText is an assistant message that ends a design run.
const RunEndText = "Done.\n✅ Map: drafted. Next: start a fresh session"

// GrillingFixture is a started grilling session: its harness, event stream,
// session and conversation ids, and transcript path.
type GrillingFixture struct {
	H    *Harness
	Ctx  context.Context
	Evs  <-chan wire.Event
	ID   string
	Conv string
	Path string
}

// EndTurn runs a working-then-idle turn on the session.
func EndTurn(t *testing.T, f *GrillingFixture) {
	t.Helper()
	Report(t, f.Conv, agentfake.Control{State: "working"})
	WaitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	Report(t, f.Conv, agentfake.Control{State: "idle"})
}

// StartGrillingSessionIn starts a fake session on a map epic (Design) or a
// plain one, on a store the caller has already seeded, with claude's config
// dir pointed at a temp dir. It returns the transcript path of its
// conversation.
func StartGrillingSessionIn(t *testing.T, ticket, store string, opts ...func(*server.Config)) (*Harness, context.Context, <-chan wire.Event, string, string, string) {
	t.Helper()
	cfg := t.TempDir()
	WriteTicket(t, store, "proj", "map", "01", "first", "")
	WriteTicket(t, store, "proj", "plain", "01", "first", "")
	// A caller may seed the map with a design_step first.
	if mapFile := filepath.Join(store, "proj", "map", "ticket.md"); !strings.Contains(string(mustRead(mapFile)), "kind: map") {
		if err := os.WriteFile(mapFile, []byte("---\nkind: map\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts = append([]func(*server.Config){WithTestHolder(), WithChatHarness(string(agentfake.HarnessKind)), WithDefaultHarness(string(agentfake.HarnessKind)), WithAlerts(true, true), WithEnv("CLAUDE_CONFIG_DIR=" + cfg)}, opts...)
	h := StartWithStore(t, store, opts...)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.CreateSession(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: ticket})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := ReadMeta(t, h, id)["conversation_id"].(string)
	return h, ctx, evs, id, conv, filepath.Join(cfg, "projects", "-repo", conv+".jsonl")
}

// mustRead is a file's content, or nil when it does not exist yet.
func mustRead(path string) []byte {
	b, _ := os.ReadFile(path)
	return b
}

// StoreWithMapStep is a ticket store whose map epic is a map at step.
func StoreWithMapStep(t *testing.T, step string) string {
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

// WriteAssistantMessage writes one assistant text message to the transcript.
func WriteAssistantMessage(t *testing.T, path, text string) {
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

// ClearByInput types a key into the session's terminal, which clears the
// raised Actionable.
func ClearByInput(t *testing.T, h *Harness, id string) {
	t.Helper()
	c, err := holder.DialHolder(filepath.Join(h.SessionsDir, id))
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
