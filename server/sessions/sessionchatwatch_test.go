package sessions_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/testutil/agentfake"
)

// These watch tests stay in the root: they drive the Server's session state
// into the chat poll, which needs the whole Server. The chat reads and the
// poll's own ticking are tested in server/sessions/chat.

// startChatSession starts a fake session whose harness counts as the chat
// harness and returns its id, conversation and transcript path.
func startChatSession(t *testing.T, kind string) (*servertest.Harness, context.Context, <-chan wire.Event, string, string, string) {
	t.Helper()
	cfg := t.TempDir()
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)), servertest.WithDefaultHarness(string(agentfake.HarnessKind)), servertest.WithChatPollInterval(chatPollTestInterval), servertest.WithEnv("CLAUDE_CONFIG_DIR="+cfg))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.CreateSession(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: kind})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, h, id)["conversation_id"].(string)
	return h, ctx, evs, id, conv, filepath.Join(cfg, "projects", "-repo", conv+".jsonl")
}

// startCodexChatSession starts a fake session and relabels it as a Codex one,
// whose rollout lives under the returned CODEX_HOME.
func startCodexChatSession(t *testing.T) (*servertest.Harness, context.Context, string, string) {
	t.Helper()
	codexHome := t.TempDir()
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)), servertest.WithDefaultHarness(string(agentfake.HarnessKind)), servertest.WithChatPollInterval(chatPollTestInterval), servertest.WithEnv("CODEX_HOME="+codexHome))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	res, err := h.Client.CreateSession(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	meta := readMeta(t, h, id)
	meta["harness"] = "codex"
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(h.SessionsDir, id, "meta.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	conv, _ := meta["conversation_id"].(string)
	return h, ctx, id, filepath.Join(codexHome, "sessions", "2026", "01", "01", "rollout-2026-01-01T10-00-00-"+conv+".jsonl")
}

func TestSessionChatChanged_WorkingSessionPublishesOnGrowth(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := startChatSession(t, string(agentfake.HarnessKind))
	writeTranscript(t, path, chatLine)
	report(t, conv, agentfake.Control{State: "working"})
	waitSessionState(ctx, t, h, id, wire.SessionWorking)
	if !chatEventWithin(evs, t, path, id, 5*time.Second, true) {
		t.Fatal("no session-chat-changed event while the transcript grew")
	}
}

func TestSessionChatChanged_UnchangedTranscriptPublishesNothing(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := startChatSession(t, string(agentfake.HarnessKind))
	writeTranscript(t, path, chatLine)
	report(t, conv, agentfake.Control{State: "working"})
	waitSessionState(ctx, t, h, id, wire.SessionWorking)
	if chatEventWithin(evs, t, path, id, 10*chatPollTestInterval, false) {
		t.Fatal("session-chat-changed fired for an unchanged transcript")
	}
}

func TestSessionChatChanged_IdleSessionPublishesNothing(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := startChatSession(t, string(agentfake.HarnessKind))
	writeTranscript(t, path, chatLine)
	report(t, conv, agentfake.Control{State: "done"})
	waitSessionState(ctx, t, h, id, wire.SessionIdle)
	// Negative wait: the meta shows Idle a moment before the watch stops its poll, and no event marks the stop.
	time.Sleep(2 * chatPollTestInterval)
	if chatEventWithin(evs, t, path, id, 10*chatPollTestInterval, true) {
		t.Fatal("session-chat-changed fired for an idle session")
	}
}

// Claude reports idle before it writes the turn's last lines, so the final
// message lands after the poll stopped.
func TestSessionChatChanged_TurnWrittenAfterIdlePublishes(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := startChatSession(t, string(agentfake.HarnessKind))
	writeTranscript(t, path, chatLine)
	report(t, conv, agentfake.Control{State: "working"})
	waitSessionState(ctx, t, h, id, wire.SessionWorking)
	report(t, conv, agentfake.Control{State: "done"})
	waitSessionState(ctx, t, h, id, wire.SessionIdle)
	// Negative wait: the meta shows Idle a moment before the watch stops its poll, and no event marks the stop.
	time.Sleep(2 * chatPollTestInterval)
	appendLines(t, path, chatLine, turnDurationLine)
	if !chatEventWithin(evs, t, path, id, time.Second, false) {
		t.Fatal("no session-chat-changed event for the turn written after idle")
	}
}
