package server_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/testutil/agentfake"
)

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

const turnDurationLine = `{"type":"system","subtype":"turn_duration","durationMs":100}`

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
