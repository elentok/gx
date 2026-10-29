package server_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

const chatPollTestInterval = 50 * time.Millisecond

const chatLine = `{"type":"assistant","uuid":"a","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"hi"}]}}`

func appendTranscript(t *testing.T, path string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(chatLine + "\n"); err != nil {
		t.Fatal(err)
	}
}

func waitSessionState(ctx context.Context, t *testing.T, h *servertest.Harness, id, state string) {
	t.Helper()
	servertest.WaitFor(t, 20*time.Second, fmt.Sprintf("session %s to reach %s", id, state), func() bool {
		list, _ := h.Client.Sessions(ctx)
		return slices.ContainsFunc(list, func(s server.SessionInfo) bool { return s.ID == id && s.Status.State == state })
	})
}

// chatEventWithin reports whether a session-chat-changed event for id arrives
// within d, appending to the transcript on every poll when grow is set.
func chatEventWithin(evs <-chan server.Event, t *testing.T, path, id string, d time.Duration, grow bool) bool {
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
			if grow {
				appendTranscript(t, path)
			}
		case <-deadline:
			return false
		}
	}
}

func TestSessionChatChanged_WorkingSessionPublishesOnGrowth(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := startChatSession(t, string(agentfake.HarnessKind))
	writeTranscript(t, path, chatLine)
	report(t, conv, agentfake.Control{State: "working"})
	waitSessionState(ctx, t, h, id, server.SessionWorking)
	if !chatEventWithin(evs, t, path, id, 5*time.Second, true) {
		t.Fatal("no session-chat-changed event while the transcript grew")
	}
}

func TestSessionChatChanged_UnchangedTranscriptPublishesNothing(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := startChatSession(t, string(agentfake.HarnessKind))
	writeTranscript(t, path, chatLine)
	report(t, conv, agentfake.Control{State: "working"})
	waitSessionState(ctx, t, h, id, server.SessionWorking)
	if chatEventWithin(evs, t, path, id, 10*chatPollTestInterval, false) {
		t.Fatal("session-chat-changed fired for an unchanged transcript")
	}
}

func TestSessionChatChanged_IdleSessionPublishesNothing(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := startChatSession(t, string(agentfake.HarnessKind))
	writeTranscript(t, path, chatLine)
	report(t, conv, agentfake.Control{State: "done"})
	waitSessionState(ctx, t, h, id, server.SessionIdle)
	// Negative wait: the meta shows Idle a moment before the watch stops its poll, and no event marks the stop.
	time.Sleep(2 * chatPollTestInterval)
	if chatEventWithin(evs, t, path, id, 10*chatPollTestInterval, true) {
		t.Fatal("session-chat-changed fired for an idle session")
	}
}

const turnDurationLine = `{"type":"system","subtype":"turn_duration","durationMs":100}`

// appendLines appends raw lines to the transcript at path.
func appendLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatal(err)
		}
	}
}

// Claude reports idle before it writes the turn's last lines, so the final
// message lands after the poll stopped.
func TestSessionChatChanged_TurnWrittenAfterIdlePublishes(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := startChatSession(t, string(agentfake.HarnessKind))
	writeTranscript(t, path, chatLine)
	report(t, conv, agentfake.Control{State: "working"})
	waitSessionState(ctx, t, h, id, server.SessionWorking)
	report(t, conv, agentfake.Control{State: "done"})
	waitSessionState(ctx, t, h, id, server.SessionIdle)
	// Negative wait: the meta shows Idle a moment before the watch stops its poll, and no event marks the stop.
	time.Sleep(2 * chatPollTestInterval)
	appendLines(t, path, chatLine, turnDurationLine)
	if !chatEventWithin(evs, t, path, id, time.Second, false) {
		t.Fatal("no session-chat-changed event for the turn written after idle")
	}
}
