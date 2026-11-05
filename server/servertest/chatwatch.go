package servertest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
)

const ChatPollTestInterval = 50 * time.Millisecond

const ChatLine = `{"type":"assistant","uuid":"a","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"hi"}]}}`

const TurnDurationLine = `{"type":"system","subtype":"turn_duration","durationMs":100}`

// CodexMessageLine is a Codex assistant message line.
const CodexMessageLine = `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}`

// WriteTranscript replaces the transcript at path with lines.
func WriteTranscript(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// AppendLines appends raw lines to the transcript at path.
func AppendLines(t *testing.T, path string, lines ...string) {
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

func AppendTranscript(t *testing.T, path string) {
	t.Helper()
	AppendLines(t, path, ChatLine)
}

func WaitSessionState(ctx context.Context, t *testing.T, h *Harness, id, state string) {
	t.Helper()
	WaitFor(t, 20*time.Second, fmt.Sprintf("session %s to reach %s", id, state), func() bool {
		list, _ := h.Client.Sessions(ctx)
		return slices.ContainsFunc(list, func(s wire.SessionInfo) bool { return s.ID == id && s.Status.State == state })
	})
}

// ChatEventWithin reports whether a session-chat-changed event for id arrives
// within d, appending to the transcript on every poll when grow is set.
func ChatEventWithin(evs <-chan wire.Event, t *testing.T, path, id string, d time.Duration, grow bool) bool {
	t.Helper()
	deadline := time.After(d)
	tick := time.NewTicker(ChatPollTestInterval)
	defer tick.Stop()
	for {
		select {
		case ev := <-evs:
			if ev.Type == wire.EventSessionChatChanged && ev.Address == id {
				return true
			}
		case <-tick.C:
			if grow {
				AppendTranscript(t, path)
			}
		case <-deadline:
			return false
		}
	}
}
