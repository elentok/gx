package sessions_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// startFailureSession starts a fake session; chat makes its harness the chat
// harness, whose transcripts auth and stalled read.
func startFailureSession(t *testing.T, chat bool, stall time.Duration) (*servertest.Harness, context.Context, <-chan wire.Event, string, string, string) {
	t.Helper()
	cfg := t.TempDir()
	opts := []func(*server.Config){servertest.WithStallThreshold(stall), servertest.WithEnv("CLAUDE_CONFIG_DIR=" + cfg)}
	if chat {
		opts = append(opts, servertest.WithChatHarness(string(agentfake.HarnessKind)), servertest.WithDefaultHarness(string(agentfake.HarnessKind)))
	}
	h, ctx, evs, id, conv := startFakeSessionWith(t, wire.CreateSessionRequest{}, opts...)
	return h, ctx, evs, id, conv, filepath.Join(cfg, "projects", "-repo", conv+".jsonl")
}

func TestSessionFailure_Kinds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		drive func(t *testing.T, conv string)
		want  wire.SessionFailure
	}{
		{"crashed", func(t *testing.T, conv string) { exitFake(t, conv, 1) }, wire.SessionFailure{Kind: "crashed", Label: "Agent crashed"}},
		{"ended", func(t *testing.T, conv string) { exitFake(t, conv, 0) }, wire.SessionFailure{Kind: "ended", Label: "Session ended"}},
		{"rate-limit", func(t *testing.T, conv string) {
			report(t, conv, agentfake.Control{State: "ratelimited", Message: "resets at 5pm"})
		}, wire.SessionFailure{Kind: "rate-limit", Label: "Rate limited", Text: "resets at 5pm"}},
		{"rate-limit without a message", func(t *testing.T, conv string) {
			report(t, conv, agentfake.Control{State: "ratelimited"})
		}, wire.SessionFailure{Kind: "rate-limit", Label: "Rate limited"}},
		{"blocked", func(t *testing.T, conv string) {
			report(t, conv, agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"})
		}, wire.SessionFailure{Kind: "blocked", Why: "permission", Label: "Waiting at a prompt in the terminal"}},
		{"blocked on a dialog", func(t *testing.T, conv string) {
			report(t, conv, agentfake.Control{State: "blocked", Kind: "dialog"})
		}, wire.SessionFailure{Kind: "blocked", Why: "dialog", Label: "Waiting at a prompt in the terminal"}},
		{"blocked on a question", func(t *testing.T, conv string) {
			report(t, conv, agentfake.Control{State: "blocked", Kind: "question"})
		}, wire.SessionFailure{Kind: "blocked", Why: "question", Label: "Waiting at a prompt in the terminal"}},
		{"agent error", func(t *testing.T, conv string) {
			report(t, conv, agentfake.Control{State: "error", Message: "model gone"})
		}, wire.SessionFailure{Kind: "blocked", Why: "error", Label: "Agent hit an error", Text: "model gone"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, ctx, _, id, conv, _ := startFailureSession(t, true, time.Hour)
			tt.drive(t, conv)
			if got := waitForFailure(ctx, t, h, id, tt.want.Kind); *got != tt.want {
				t.Errorf("failure = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func exitFake(t *testing.T, conv string, code int) {
	t.Helper()
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionExit, Code: code}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionFailure_ClearsWhenTheStatusChangesBack(t *testing.T) {
	t.Parallel()
	h, ctx, _, id, conv, _ := startFailureSession(t, true, time.Hour)
	report(t, conv, agentfake.Control{State: "ratelimited"})
	waitForFailure(ctx, t, h, id, "rate-limit")
	report(t, conv, agentfake.Control{State: "working"})
	waitForFailure(ctx, t, h, id, "")
}

func TestSessionFailure_AuthFromTheLastAPIErrorWhileIdle(t *testing.T) {
	t.Parallel()
	h, ctx, _, id, conv, path := startFailureSession(t, true, time.Hour)
	writeTranscript(t, path, apiError401)
	report(t, conv, agentfake.Control{State: "done"})
	got := waitForFailure(ctx, t, h, id, "auth")
	want := wire.SessionFailure{Kind: "auth", Label: "Claude login needed", Text: "Run `claude /login` on this computer, then Retry."}
	if *got != want {
		t.Errorf("failure = %+v, want %+v", *got, want)
	}
	writeTranscript(t, path, apiError401, reply)
	waitForFailure(ctx, t, h, id, "")
}

func TestSessionFailure_StalledWhenTheTranscriptIsOldWhileWorking(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := startFailureSession(t, true, time.Minute)
	writeTranscript(t, path, reply)
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	if list, chat := failureNow(ctx, t, h, id); list != nil || chat != nil {
		t.Fatalf("failure = %+v / %+v, want none while the transcript is fresh", list, chat)
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	got := waitForFailure(ctx, t, h, id, "stalled")
	if want := (wire.SessionFailure{Kind: "stalled", Label: "No progress for 15 min"}); *got != want {
		t.Errorf("failure = %+v, want %+v", *got, want)
	}
	writeTranscript(t, path, reply, reply)
	waitForFailure(ctx, t, h, id, "")
}

func TestSessionFailure_ARestoreCrashingWithinAMinuteIsReported(t *testing.T) {
	t.Parallel()
	h, ctx, _, id, conv, _ := startFailureSession(t, true, time.Hour)
	exitFake(t, conv, 1)
	if got := waitForFailure(ctx, t, h, id, "crashed"); got.Recrashed {
		t.Fatalf("failure = %+v, want the first crash not to be a re-crash", got)
	}
	if res, err := h.Client.ResumeSession(ctx, id); err != nil || res.Refused {
		t.Fatalf("restore = %+v, %v", res, err)
	}
	waitForFailure(ctx, t, h, id, "")
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	exitFake(t, conv, 1)
	if got := waitForFailure(ctx, t, h, id, "crashed"); !got.Recrashed {
		t.Errorf("failure = %+v, want a re-crash", got)
	}
}

func TestSessionFailure_NonChatHarnessNeverGetsAuthOrStalled(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv, path := startFailureSession(t, false, time.Minute)
	writeTranscript(t, path, apiError401)
	report(t, conv, agentfake.Control{State: "done"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	if list, chat := failureNow(ctx, t, h, id); list != nil || chat != nil {
		t.Errorf("idle failure = %+v / %+v, want none", list, chat)
	}

	writeTranscript(t, path, reply)
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	if list, chat := failureNow(ctx, t, h, id); list != nil || chat != nil {
		t.Errorf("working failure = %+v / %+v, want none", list, chat)
	}
}
