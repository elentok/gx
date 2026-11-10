package herdrrunner_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/testutil/herdrfake"
)

// The pane's shell gets the cap before the agent starts in it.
func TestStart_CapsThePaneBeforeTheAgentStarts(t *testing.T) {
	state := herdrfake.NewState(t)
	_, fakeEnv := herdrfake.StartAgentHostEnv(t, state)
	r := newTestRunner(fakeEnv)
	r.ProcCap = func(*slog.Logger) int { return 1234 }
	if _, err := r.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	var seq []string
	for _, e := range state.Trace() {
		if len(e.Argv) >= 2 && (e.Argv[0] == "pane" && e.Argv[1] == "run" || e.Argv[0] == "agent" && e.Argv[1] == "start") {
			seq = append(seq, strings.Join(e.Argv[:2], " ")+": "+strings.Join(e.Argv[2:], " "))
		}
	}
	if len(seq) != 2 || !strings.HasPrefix(seq[0], "pane run: ") || !strings.HasSuffix(seq[0], " ulimit -u 1234") || !strings.HasPrefix(seq[1], "agent start: ") {
		t.Fatalf("pane commands = %q, want the ulimit run, then agent start", seq)
	}
}

const launchText = "/gx-implement gx:epic/01"

// startLaunch is a Claude session whose transcripts live under a temp home.
func startLaunch(t *testing.T) (*herdrrunner.Runner, *herdrfake.State, agentrunner.Session, string) {
	t.Helper()
	r, state, s := startAgent(t, "claude", nil)
	r.Home = t.TempDir()
	r.N = 50 * time.Millisecond
	r.LaunchCaps = claudeui.LaunchProofCaps{
		Grace: 300 * time.Millisecond, UnknownGrace: 100 * time.Millisecond,
		Poll: 5 * time.Millisecond, ResendEvery: 100 * time.Millisecond,
	}
	s.SessionID = "sess-1"
	return r, state, s, r.Home
}

func writeFirstUser(t *testing.T, home, content string) {
	t.Helper()
	path := filepath.Join(home, ".claude", "projects", "proj", "sess-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","uuid":"u0","timestamp":"2026-01-01T10:00:00Z","message":{"content":"` + content + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

const launchRecord = `<command-name>/gx-implement</command-name>\n<command-args>gx:epic/01</command-args>`

func TestPromptLaunch_WorkingAndTranscriptAgreeIsDeliveredOnce(t *testing.T) {
	r, state, s, home := startLaunch(t)
	writeFirstUser(t, home, launchRecord)
	if err := state.SlashStartsTurn(s.Label); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), r, s, launchText, nil); err != nil {
		t.Fatalf("PromptLaunch = %v, want nil", err)
	}
	if got, _ := state.Received(s.Label); !slices.Equal(got, []string{launchText}) {
		t.Fatalf("received = %q, want one send", got)
	}
}

func TestPromptLaunch_NeverWorkingIsTheStalledPromptError(t *testing.T) {
	r, state, s, home := startLaunch(t)
	// Even a transcript showing the command does not outvote herdr.
	writeFirstUser(t, home, launchRecord)
	if err := state.StallAgent(s.Label); err != nil {
		t.Fatal(err)
	}
	err := agentrunner.PromptLaunch(context.Background(), r, s, launchText, nil)
	if !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v, want ErrNotDelivered", err)
	}
	if !herdrrunner.IsPollTimeout(err) {
		t.Fatalf("PromptLaunch = %v, want herdr's stalled-prompt text kept", err)
	}
}

func TestPromptLaunch_WorkingWithoutTranscriptIsNotDelivered(t *testing.T) {
	r, state, s, _ := startLaunch(t)
	if err := state.SlashStartsTurn(s.Label); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), r, s, launchText, nil); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v, want ErrNotDelivered", err)
	}
}

func TestPromptLaunch_TurnPassedUnseenNeedsTheTranscript(t *testing.T) {
	r, state, s, home := startLaunch(t)
	if err := state.FastTurn(s.Label); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), r, s, launchText, nil); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch without a transcript = %v, want ErrNotDelivered", err)
	}

	r, state, s, home = startLaunch(t)
	writeFirstUser(t, home, launchRecord)
	if err := state.FastTurn(s.Label); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), r, s, launchText, nil); err != nil {
		t.Fatalf("PromptLaunch with a transcript = %v, want nil", err)
	}
}

func TestPromptLaunch_UnknownCommandOnThePaneIsNotDelivered(t *testing.T) {
	r, state, s, _ := startLaunch(t)
	if err := state.SetPaneText(s.Label, "Unknown command: /gx-implement"); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), r, s, launchText, nil); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v, want ErrNotDelivered", err)
	}
}

// A slash command that is not a launch prompt keeps the single-send rule.
func TestPrompt_ClaudeSlashCommandIsStillSentOnce(t *testing.T) {
	r, state, s, _ := startLaunch(t)
	if err := r.Prompt(s, "/status"); err != nil {
		t.Fatalf("Prompt(/status) = %v, want nil", err)
	}
	if got, _ := state.Received(s.Label); !slices.Equal(got, []string{"/status"}) {
		t.Fatalf("received = %q, want one send", got)
	}
}

// startCodexLaunch is a Codex session whose rollouts live under a temp home.
func startCodexLaunch(t *testing.T) (*herdrrunner.Runner, *herdrfake.State, agentrunner.Session) {
	t.Helper()
	r, state, s := startCodex(t, nil)
	r.CodexHome = t.TempDir()
	r.N = 50 * time.Millisecond
	r.LaunchCaps = claudeui.LaunchProofCaps{
		Grace: 300 * time.Millisecond, UnknownGrace: 100 * time.Millisecond,
		Poll: 5 * time.Millisecond, ResendEvery: 100 * time.Millisecond,
	}
	s.SessionID = "sess-codex"
	return r, state, s
}

func writeCodexUser(t *testing.T, home, text string) {
	t.Helper()
	path := filepath.Join(home, "sessions", "2026", "08", "04", "rollout-2026-08-04T00-00-00-sess-codex.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + text + `"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPromptLaunch_CodexRolloutRecordIsDelivered(t *testing.T) {
	r, state, s := startCodexLaunch(t)
	// Codex writes the record after the send, so the offset taken before it
	// must not hide it.
	writeCodexUser(t, r.CodexHome, "an older prompt")
	go func() {
		for {
			if got, _ := state.Received(s.Label); len(got) > 0 {
				break
			}
			runtime.Gosched()
		}
		path, _, _ := codexsession.RolloutPathIn(r.CodexHome, s.SessionID)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = f.WriteString(`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + launchText + `"}]}}` + "\n")
			_ = f.Close()
		}
	}()
	if err := agentrunner.PromptLaunch(context.Background(), r, s, launchText, nil); err != nil {
		t.Fatalf("PromptLaunch = %v, want nil", err)
	}
	if got, _ := state.Received(s.Label); !slices.Equal(got, []string{launchText}) {
		t.Fatalf("received = %q, want one send", got)
	}
}

func TestPromptLaunch_CodexWithoutARecordResendsAndIsNotDelivered(t *testing.T) {
	r, state, s := startCodexLaunch(t)
	var nudges []string
	err := agentrunner.PromptLaunch(context.Background(), r, s, launchText, func(d string) { nudges = append(nudges, d) })
	if !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v, want ErrNotDelivered", err)
	}
	if got, _ := state.Received(s.Label); len(got) < 2 {
		t.Fatalf("received = %q, want a resend", got)
	}
	if len(nudges) == 0 {
		t.Fatal("onNudge never called for the resends")
	}
}

// Codex launches without a Codex home keep the unproved single send.
func TestPromptLaunch_CodexIsUnchanged(t *testing.T) {
	r, state, s := startCodex(t, nil)
	r.Home = t.TempDir()
	r.N = 50 * time.Millisecond
	if err := agentrunner.PromptLaunch(context.Background(), r, s, "/status", nil); err != nil {
		t.Fatalf("PromptLaunch = %v, want nil", err)
	}
	if got, _ := state.Received(s.Label); !slices.Equal(got, []string{"/status"}) {
		t.Fatalf("received = %q, want one send", got)
	}
}
