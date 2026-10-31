package server_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

const codexMessageLine = `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}`

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
	waitSessionState(ctx, t, h, id, server.SessionWorking)
	if !codexEventWithin(evs, t, path, id, 5*time.Second) {
		t.Fatal("no session-chat-changed event while the rollout grew")
	}
}

func codexEventWithin(evs <-chan server.Event, t *testing.T, path, id string, d time.Duration) bool {
	t.Helper()
	deadline := time.After(d)
	tick := time.NewTicker(chatPollTestInterval)
	defer tick.Stop()
	for {
		select {
		case ev := <-evs:
			if ev.Type == server.EventSessionChatChanged && ev.Address == id {
				return true
			}
		case <-tick.C:
			appendLines(t, path, codexMessageLine)
		case <-deadline:
			return false
		}
	}
}
