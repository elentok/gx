package herdrrunner_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/testutil/herdrfake"
)

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
	if err := agentrunner.PromptLaunch(r, s, launchText, nil); err != nil {
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
	err := agentrunner.PromptLaunch(r, s, launchText, nil)
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
	if err := agentrunner.PromptLaunch(r, s, launchText, nil); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v, want ErrNotDelivered", err)
	}
}

func TestPromptLaunch_TurnPassedUnseenNeedsTheTranscript(t *testing.T) {
	r, state, s, home := startLaunch(t)
	if err := state.FastTurn(s.Label); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(r, s, launchText, nil); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch without a transcript = %v, want ErrNotDelivered", err)
	}

	r, state, s, home = startLaunch(t)
	writeFirstUser(t, home, launchRecord)
	if err := state.FastTurn(s.Label); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(r, s, launchText, nil); err != nil {
		t.Fatalf("PromptLaunch with a transcript = %v, want nil", err)
	}
}

func TestPromptLaunch_UnknownCommandOnThePaneIsNotDelivered(t *testing.T) {
	r, state, s, _ := startLaunch(t)
	if err := state.SetPaneText(s.Label, "Unknown command: /gx-implement"); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(r, s, launchText, nil); !errors.Is(err, agentrunner.ErrNotDelivered) {
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

// Codex launches keep the unproved single send.
func TestPromptLaunch_CodexIsUnchanged(t *testing.T) {
	r, state, s := startCodex(t, nil)
	r.Home = t.TempDir()
	r.N = 50 * time.Millisecond
	if err := agentrunner.PromptLaunch(r, s, "/status", nil); err != nil {
		t.Fatalf("PromptLaunch = %v, want nil", err)
	}
	if got, _ := state.Received(s.Label); !slices.Equal(got, []string{"/status"}) {
		t.Fatalf("received = %q, want one send", got)
	}
}
