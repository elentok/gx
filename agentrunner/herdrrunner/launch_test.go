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

type launchFixture struct {
	t       *testing.T
	r       *herdrrunner.Runner
	state   *herdrfake.State
	session agentrunner.Session
	home    string
}

// startLaunch is a Claude session whose transcripts live under a temp home.
func startLaunch(t *testing.T) *launchFixture {
	t.Helper()
	r, state, s := startAgent(t, "claude", nil)
	r.Home = t.TempDir()
	r.N = 50 * time.Millisecond
	r.LaunchCaps = claudeui.LaunchProofCaps{
		Grace: 300 * time.Millisecond, UnknownGrace: 100 * time.Millisecond,
		Poll: 5 * time.Millisecond, ResendEvery: 100 * time.Millisecond,
	}
	s.SessionID = "sess-1"
	return &launchFixture{t: t, r: r, state: state, session: s, home: r.Home}
}

// writeFirstUser makes the launch record land once the trace shows the prompt
// call, so after the proof's pre-send count, as a real send does.
func (f *launchFixture) writeFirstUser(content string) {
	f.t.Helper()
	done := make(chan struct{})
	f.t.Cleanup(func() { <-done })
	go func() {
		defer close(done)
		tick := time.NewTicker(2 * time.Millisecond)
		defer tick.Stop()
		timeout := time.After(2 * time.Second)
		for {
			select {
			case <-timeout:
				return
			case <-tick.C:
			}
			for _, e := range f.state.Trace() {
				if len(e.Argv) >= 2 && e.Argv[0] == "agent" && e.Argv[1] == "prompt" {
					appendUser(f.home, "u1", content)
					return
				}
			}
		}
	}()
}

func appendUser(home, uuid, content string) {
	path := filepath.Join(home, ".claude", "projects", "proj", "sess-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	line := `{"type":"user","uuid":"` + uuid + `","timestamp":"2026-01-01T10:00:00Z","message":{"content":"` + content + `"}}` + "\n"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

// A resumed session's old record equal to the command never proves a new launch.
func TestPromptLaunch_ResumedTranscriptsOldLaunchRecordIsNotTheProof(t *testing.T) {
	f := startLaunch(t)
	appendUser(f.home, "u0", launchRecord)

	err := agentrunner.PromptLaunch(context.Background(), f.r, f.session, launchText, nil)

	if !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v, want ErrNotDelivered", err)
	}
}

const launchRecord = `<command-name>/gx-implement</command-name>\n<command-args>gx:epic/01</command-args>`

func TestPromptLaunch_WorkingAndTranscriptAgreeIsDeliveredOnce(t *testing.T) {
	f := startLaunch(t)
	f.writeFirstUser(launchRecord)
	if err := f.state.SlashStartsTurn(f.session.Label); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), f.r, f.session, launchText, nil); err != nil {
		t.Fatalf("PromptLaunch = %v, want nil", err)
	}
	if got, _ := f.state.Received(f.session.Label); !slices.Equal(got, []string{launchText}) {
		t.Fatalf("received = %q, want one send", got)
	}
}

func TestPromptLaunch_NeverWorkingIsTheStalledPromptError(t *testing.T) {
	f := startLaunch(t)
	// Even a transcript showing the command does not outvote herdr.
	f.writeFirstUser(launchRecord)
	if err := f.state.StallAgent(f.session.Label); err != nil {
		t.Fatal(err)
	}
	err := agentrunner.PromptLaunch(context.Background(), f.r, f.session, launchText, nil)
	if !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v, want ErrNotDelivered", err)
	}
	if !herdrrunner.IsPollTimeout(err) {
		t.Fatalf("PromptLaunch = %v, want herdr's stalled-prompt text kept", err)
	}
}

func TestPromptLaunch_WorkingWithoutTranscriptIsNotDelivered(t *testing.T) {
	f := startLaunch(t)
	if err := f.state.SlashStartsTurn(f.session.Label); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), f.r, f.session, launchText, nil); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v, want ErrNotDelivered", err)
	}
}

func TestPromptLaunch_TurnPassedUnseenNeedsTheTranscript(t *testing.T) {
	f := startLaunch(t)
	if err := f.state.FastTurn(f.session.Label); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), f.r, f.session, launchText, nil); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch without a transcript = %v, want ErrNotDelivered", err)
	}

	f = startLaunch(t)
	f.writeFirstUser(launchRecord)
	if err := f.state.FastTurn(f.session.Label); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), f.r, f.session, launchText, nil); err != nil {
		t.Fatalf("PromptLaunch with a transcript = %v, want nil", err)
	}
}

func TestPromptLaunch_UnknownCommandOnThePaneIsNotDelivered(t *testing.T) {
	f := startLaunch(t)
	if err := f.state.SetPaneText(f.session.Label, "Unknown command: /gx-implement"); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.PromptLaunch(context.Background(), f.r, f.session, launchText, nil); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("PromptLaunch = %v, want ErrNotDelivered", err)
	}
}

// A slash command that is not a launch prompt keeps the single-send rule.
func TestPrompt_ClaudeSlashCommandIsStillSentOnce(t *testing.T) {
	f := startLaunch(t)
	if err := f.r.Prompt(f.session, "/status"); err != nil {
		t.Fatalf("Prompt(/status) = %v, want nil", err)
	}
	if got, _ := f.state.Received(f.session.Label); !slices.Equal(got, []string{"/status"}) {
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
