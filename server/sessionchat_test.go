package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// startChatSession starts a fake session whose harness counts as the chat
// harness and returns its id, conversation and transcript path.
func startChatSession(t *testing.T, kind string) (*servertest.Harness, context.Context, <-chan server.Event, string, string, string) {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: kind})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, h, id)["conversation_id"].(string)
	return h, ctx, evs, id, conv, filepath.Join(cfg, "projects", "-repo", conv+".jsonl")
}

func writeTranscript(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSessionChat_ShowsVisibleMessagesInOrder(t *testing.T) {
	h, ctx, _, id, conv, path := startChatSession(t, string(agentfake.HarnessKind))
	writeTranscript(t, path,
		`{"type":"user","uuid":"u1","timestamp":"2026-01-01T10:00:00Z","message":{"content":"<command-name>/gx-design</command-name><command-args>proj:map</command-args>"}}`,
		`{"type":"user","uuid":"u2","isMeta":true,"timestamp":"2026-01-01T10:00:01Z","message":{"content":"caveat"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:02Z","message":{"content":[{"type":"thinking","thinking":"hmm"}]}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:03Z","message":{"content":[{"type":"text","text":"Which store?"}]}}`,
		`{"type":"user","uuid":"u3","timestamp":"2026-01-01T10:00:04Z","message":{"content":[{"type":"tool_result","tool_use_id":"t","content":"x"}]}}`,
		`{"type":"user","uuid":"u4","timestamp":"2026-01-01T10:00:05Z","message":{"content":"SQLite"}}`,
		`{"type":"system","uuid":"y1","timestamp":"2026-01-01T10:00:06Z","content":"note"}`,
	)
	got, err := h.Client.SessionChat(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConversationID != conv || got.Unsupported {
		t.Fatalf("chat = %+v, want conversation %s", got, conv)
	}
	var ids []string
	for _, m := range got.Messages {
		ids = append(ids, m.ID+":"+m.Role+":"+m.Kind+":"+m.Text)
	}
	want := "u1:person:command:/gx-design proj:map,a2:agent:text:Which store?,u4:person:prompt:SQLite"
	if strings.Join(ids, ",") != want {
		t.Errorf("messages = %v, want %s", ids, want)
	}
}

func TestSessionChat_OnlyTheNewestConversation(t *testing.T) {
	h, ctx, evs, id, _, path := startChatSession(t, string(agentfake.HarnessKind))
	writeTranscript(t, path, `{"type":"assistant","uuid":"old","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"before clear"}]}}`)
	if err := agentfake.SendProgramControl(readMeta(t, h, id)["conversation_id"].(string), agentfake.Control{Action: agentfake.ActionConversation, ConversationID: "conv-2"}); err != nil {
		t.Fatal(err)
	}
	waitForSession(ctx, t, h, evs, id, func(server.SessionInfo) bool { return readMeta(t, h, id)["conversation_id"] == "conv-2" })
	writeTranscript(t, filepath.Join(filepath.Dir(path), "conv-2.jsonl"),
		`{"type":"assistant","uuid":"new","timestamp":"2026-01-01T10:01:00Z","message":{"content":[{"type":"text","text":"after clear"}]}}`)
	got, err := h.Client.SessionChat(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConversationID != "conv-2" || len(got.Messages) != 1 || got.Messages[0].ID != "new" {
		t.Errorf("chat = %+v, want only conv-2's message", got)
	}
}

func TestSessionChat_NonChatHarnessIsUnsupported(t *testing.T) {
	h, ctx, _, id, _, _ := startChatSession(t, string(agentfake.NoResumeHarnessKind))
	got, err := h.Client.SessionChat(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Unsupported || len(got.Messages) != 0 {
		t.Errorf("chat = %+v, want unsupported and empty", got)
	}
}
