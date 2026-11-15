package sessions_test

import (
	"context"
	"encoding/json"
	"fmt"
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
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: kind})
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
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
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

// A Codex session is watched by its own recorded harness, whatever the chat
// harness setting says.
func TestSessionChatChanged_CodexRolloutGrowthPublishes(t *testing.T) {
	t.Parallel()
	h, ctx, id, path := startCodexChatSession(t)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, path, codexMessageLine)
	conv, _ := readMeta(t, h, id)["conversation_id"].(string)
	report(t, conv, agentfake.Control{State: "working"})
	waitSessionState(ctx, t, h, id, wire.SessionWorking)
	if !codexEventWithin(evs, t, path, id, 5*time.Second) {
		t.Fatal("no session-chat-changed event while the rollout grew")
	}
}

// assistantAt is a Claude assistant line stamped now; extra adds JSON fields.
func assistantAt(extra string) string {
	return fmt.Sprintf(`{"type":"assistant","uuid":"a","timestamp":%q,"message":{"model":"<synthetic>","content":[{"type":"text","text":"x"}]}%s}`, time.Now().UTC().Format(time.RFC3339Nano), extra)
}

func apiErrorLine(code string) string {
	return assistantAt(fmt.Sprintf(`,"isApiErrorMessage":true,"error":%q`, code))
}

// switchAndIdle switches an idle session to model and brings the new agent idle,
// which leaves the switch unconfirmed.
func switchAndIdle(t *testing.T, h *servertest.Harness, ctx context.Context, id, conv, model string) {
	t.Helper()
	if res, err := h.Client.Sessions.Switch(ctx, wire.SessionSwitchRequest{ID: id, Model: model}); err != nil || res.Refused {
		t.Fatalf("switch = %+v, %v", res, err)
	}
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	waitFor(t, func() bool {
		return readMeta(t, h, id)["model"] == model && readMeta(t, h, id)["switch_unconfirmed"] == true
	})
}

// startUnconfirmed is an idle fake session switched to opus, with its transcript path.
func startUnconfirmed(t *testing.T) (*servertest.Harness, context.Context, <-chan wire.Event, string, string, string, map[string]any) {
	t.Helper()
	cfg := t.TempDir()
	h, ctx, evs, id, conv := startIdleSwitchable(t, servertest.WithEnv("CLAUDE_CONFIG_DIR="+cfg))
	before := readMeta(t, h, id)
	switchAndIdle(t, h, ctx, id, conv, "opus")
	return h, ctx, evs, id, conv, filepath.Join(cfg, "projects", "-repo", conv+".jsonl"), before
}

// turnWith runs one turn that writes lines.
func turnWith(t *testing.T, h *servertest.Harness, ctx context.Context, id, conv, path string, lines ...string) {
	t.Helper()
	report(t, conv, agentfake.Control{State: "working"})
	waitSessionState(ctx, t, h, id, wire.SessionWorking)
	writeTranscript(t, path, append(lines, turnDurationLine)...)
	report(t, conv, agentfake.Control{State: "idle"})
}

func TestSwitchConfirm_ModelNotFoundRollsBackToTheOldRecord(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path, before := startUnconfirmed(t)
	turnWith(t, h, ctx, id, conv, path, apiErrorLine("model_not_found"))
	waitFor(t, func() bool { return readMeta(t, h, id)["model"] == before["model"] })
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool {
		return s.Status.State == wire.SessionIdle && s.SwitchingModel == "" && s.SwitchFailedModel != ""
	})
	meta := readMeta(t, h, id)
	if meta["switch_unconfirmed"] != nil || meta["switched_by_hand"] != nil || meta["effort"] != before["effort"] || meta["switched_at"] == before["switched_at"] {
		t.Errorf("meta after rollback = %v, want the record before the switch with a new switch time", meta)
	}
	if got.SwitchFailedModel != "opus" || got.SwitchFailedOld != before["model"] || got.Model == "<synthetic>" || got.Model != before["model"] || got.ModelSwitched {
		t.Errorf("session = %+v, want the failed switch to opus and the old model %v shown", got, before["model"])
	}
}

func TestSwitchConfirm_OtherFirstRepliesKeepTheSwitch(t *testing.T) {
	t.Parallel()
	// The code of the first reply's error; empty is a normal reply. The line is
	// stamped after the switch, so it is built once the switch is made.
	cases := map[string]string{
		"reply":          "",
		"rate limit":     "rate_limit",
		"invalid":        "invalid_request",
		"unknown":        "unknown",
		"overloaded":     "overloaded",
		"authentication": "authentication_error",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, ctx, _, id, conv, path, _ := startUnconfirmed(t)
			line := assistantAt("")
			if code != "" {
				line = apiErrorLine(code)
			}
			turnWith(t, h, ctx, id, conv, path, line)
			waitFor(t, func() bool { return readMeta(t, h, id)["switch_unconfirmed"] == nil })
			meta := readMeta(t, h, id)
			if meta["model"] != "opus" || meta["switched_by_hand"] != true {
				t.Errorf("meta = %v, want the switch to opus kept", meta)
			}
		})
	}
}

// A turn that reaches no model keeps the window open, and a second switch
// rolls back to the last confirmed record rather than to the first switch.
func TestSwitchConfirm_WindowStaysOpenAndSecondSwitchKeepsTheTarget(t *testing.T) {
	t.Parallel()
	h, ctx, _, id, conv, path, before := startUnconfirmed(t)
	turnWith(t, h, ctx, id, conv, path, fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":"/clear"}}`, time.Now().UTC().Format(time.RFC3339Nano)))
	waitSessionState(ctx, t, h, id, wire.SessionIdle)
	time.Sleep(500 * time.Millisecond) // negative wait: the check runs within a poll of the turn's end line
	if meta := readMeta(t, h, id); meta["switch_unconfirmed"] != true || meta["model"] != "opus" {
		t.Fatalf("meta = %v, want the switch still unconfirmed", meta)
	}
	switchAndIdle(t, h, ctx, id, conv, "sonnet")
	turnWith(t, h, ctx, id, conv, path, apiErrorLine("model_not_found"))
	waitFor(t, func() bool { return readMeta(t, h, id)["model"] == before["model"] })
}

func TestSwitchConfirm_RestartBetweenIdleAndFirstMessageStillRollsBack(t *testing.T) {
	t.Parallel()
	h, _, _, id, _, path, before := startUnconfirmed(t)
	writeTranscript(t, path, apiErrorLine("model_not_found"), turnDurationLine)
	h.Restart(t)
	waitFor(t, func() bool { return readMeta(t, h, id)["model"] == before["model"] })
}
