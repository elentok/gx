package server_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/testutil/agentfake"
)

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
