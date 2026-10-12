package nativerunner_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/runnertest"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

// newPty returns a pty runner whose holder is this test binary and whose
// claude is the TUI fake, with claude's home in a temp dir.
func newPty(t *testing.T) *nativerunner.Pty {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := shortTempDir(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv(holderEnv, "1")
	t.Setenv(agentfake.TUIEnv, "1")
	t.Cleanup(func() { killAgents(root) })
	return &nativerunner.Pty{
		Root:         root,
		Holder:       []string{exe},
		Claude:       exe,
		PollInterval: 10 * time.Millisecond,
		StartTimeout: 10 * time.Second,
	}
}

func TestPty_Conformance(t *testing.T) {
	runnertest.Run(t, func(t *testing.T) runnertest.Harness {
		return runnertest.Harness{
			Runner: newPty(t),
			Block: func(t *testing.T, s agentrunner.Session, reason string) {
				ptyControl(t, s, agentfake.Control{Action: agentfake.ActionBlock, WaitingFor: reason})
			},
			Skip: map[string]string{
				"PromptStartsTurn":             "pty prompt lands in ticket 23",
				"PromptNotDelivered":           "pty prompt lands in ticket 23",
				"WaitTimeout":                  "pty prompt lands in ticket 23",
				"InterruptKeepsSession":        "pty interrupt lands in ticket 24",
				"CompactAfterInterrupt":        "pty interrupt and compact land in ticket 24",
				"FinishWaitsForBackgroundTask": "pty background tasks land in ticket 24",
				"BlockedThenAnswer":            "pty answer lands in ticket 30",
				"AnswerNotBlocked":             "pty answer lands in ticket 30",
				"RestartThenFind":              "pty reattach lands in ticket 26",
				"TurnSurvivesRestart":          "pty reattach lands in ticket 26",
			},
		}
	})
}

// ptyControl sends c to the TUI fake behind s.
func ptyControl(t *testing.T, s agentrunner.Session, c agentfake.Control) {
	t.Helper()
	if err := agentfake.SendTUIControl(os.Getenv("HOME"), s.SessionID, c); err != nil {
		t.Fatalf("control %s: %v", c.Action, err)
	}
}

func TestPty_StartAnswersTrustDialog(t *testing.T) {
	p := newPty(t)
	t.Setenv(agentfake.TUITrustEnv, "1")
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(s)
	if st, err := p.Status(s); err != nil || st.State != agentrunner.StateIdle {
		t.Errorf("Status = %+v, %v; want idle", st, err)
	}
}

func TestPty_StartWithoutPidFileFailsWithScreen(t *testing.T) {
	p := newPty(t)
	p.StartTimeout = 500 * time.Millisecond
	t.Setenv(agentfake.TUINoPidFileEnv, "1")
	_, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if !errors.Is(err, agentrunner.ErrMissingCapability) || !strings.Contains(err.Error(), "Welcome to Claude Code") {
		t.Fatalf("Start = %v; want ErrMissingCapability with the screen", err)
	}
	if _, ok, _ := p.Find("e-01"); ok {
		t.Error("failed Start left a session behind")
	}
}

func TestPty_BlockedReasonIsWaitingFor(t *testing.T) {
	p := newPty(t)
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(s)
	ptyControl(t, s, agentfake.Control{Action: agentfake.ActionBlock, WaitingFor: "approve Bash"})
	st, err := p.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second)
	if err != nil || st.BlockedReason != "approve Bash" {
		t.Fatalf("Wait = %+v, %v; want blocked on approve Bash", st, err)
	}
}

func TestPty_StartRecordsBothPidsAndLaunchArgs(t *testing.T) {
	p := newPty(t)
	cwd := t.TempDir()
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: cwd, Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(s)

	data, err := os.ReadFile(filepath.Join(p.Root, "e-01", nativerunner.MetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var meta nativerunner.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Runner != "pty" || meta.SessionID != s.SessionID || meta.Cwd != cwd {
		t.Errorf("meta = %+v; want runner pty, session %s, cwd %s", meta, s.SessionID, cwd)
	}
	for _, want := range []struct {
		name  string
		pid   int
		start string
	}{
		{"claude", meta.PID, meta.PIDStart},
		{"holder", meta.HolderPID, meta.HolderPIDStart},
	} {
		proc, ok, err := nativerunner.PS.Lookup(want.pid)
		if err != nil || !ok {
			t.Fatalf("%s pid %d not running: %v", want.name, want.pid, err)
		}
		if proc.Start != want.start {
			t.Errorf("%s start = %q, want %q", want.name, want.start, proc.Start)
		}
		if !strings.Contains(proc.Cmdline, s.SessionID) {
			t.Errorf("%s cmdline %q lacks the session id", want.name, proc.Cmdline)
		}
	}
	claude, _, _ := nativerunner.PS.Lookup(meta.PID)
	if !strings.Contains(claude.Cmdline, "--disallowedTools AskUserQuestion") {
		t.Errorf("claude cmdline %q does not disallow AskUserQuestion", claude.Cmdline)
	}
}

func TestPty_StopRemovesHolderSock(t *testing.T) {
	p := newPty(t)
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(p.Root, "e-01", nativerunner.HolderSocketFile)
	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("holder.sock while running: %v", err)
	}
	if err := p.Stop(s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sock); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("holder.sock after Stop: %v; want gone", err)
	}
}

func TestPty_CleanupRemovesStaleHolderSock(t *testing.T) {
	p := &nativerunner.Pty{Root: t.TempDir()}
	dir := filepath.Join(p.Root, "e-01")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, nativerunner.HolderSocketFile)
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := agentrunner.Cleanup(p, "e-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sock); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("holder.sock after Cleanup: %v; want gone", err)
	}
}
