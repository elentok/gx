package rigs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// RunEndText is an assistant message that ends a design run.
const RunEndText = "Done.\n✅ Map: drafted. Next: start a fresh session"

// GrillingFixture is a started grilling session: its harness, event stream,
// session and conversation ids, and transcript path.
type GrillingFixture struct {
	H    *servertest.Harness
	Ctx  context.Context
	Evs  <-chan wire.Event
	ID   string
	Conv string
	Path string
}

// EndTurn runs a working-then-idle turn on the session.
func EndTurn(t *testing.T, f *GrillingFixture) {
	t.Helper()
	servertest.Report(t, f.Conv, agentfake.Control{State: "working"})
	servertest.WaitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	servertest.Report(t, f.Conv, agentfake.Control{State: "idle"})
}

// StartGrillingSessionIn starts a fake session on a map epic (Design) or a
// plain one, on a store the caller has already seeded, with claude's config
// dir pointed at a temp dir. It returns the transcript path of its
// conversation.
func StartGrillingSessionIn(t *testing.T, ticket, store string, opts ...func(*server.Config)) (*servertest.Harness, context.Context, <-chan wire.Event, string, string, string) {
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
	opts = append([]func(*server.Config){servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)), servertest.WithDefaultHarness(string(agentfake.HarnessKind)), servertest.WithAlerts(true, true), servertest.WithEnv("CLAUDE_CONFIG_DIR=" + cfg)}, opts...)
	h := servertest.StartWithStore(t, store, opts...)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: ticket})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := servertest.ReadMeta(t, h, id)["conversation_id"].(string)
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
func ClearByInput(t *testing.T, h *servertest.Harness, id string) {
	t.Helper()
	c, err := holder.DialHolder(filepath.Join(h.SessionsDir, id))
	if err != nil {
		t.Fatal(err)
	}
	// A session with a first prompt blocks a person's keys until that prompt is
	// sent; these sessions never get theirs, so end the launch hold here.
	if err := c.Batch(func() error { return nil }); err != nil {
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

// WriteRounds writes n Grilling rounds to the transcript, each but the last
// answered by the person.
func WriteRounds(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	base := time.Now().Add(-time.Hour)
	for i := 1; i <= n; i++ {
		stamp := base.Add(time.Duration(i) * time.Minute).UTC().Format(time.RFC3339Nano)
		lines := []map[string]any{{"type": "assistant", "uuid": fmt.Sprintf("round-%d", i), "timestamp": stamp, "message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": fmt.Sprintf("❓ **Q1** - **Store**: Which store for round %d?", i)}}}}}
		if i < n {
			lines = append(lines, map[string]any{"type": "user", "uuid": fmt.Sprintf("answer-%d", i), "timestamp": stamp, "message": map[string]any{"role": "user", "content": "the first"}})
		}
		for _, line := range lines {
			if err := enc.Encode(line); err != nil {
				t.Fatal(err)
			}
		}
	}
}
