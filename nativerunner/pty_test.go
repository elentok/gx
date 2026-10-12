package nativerunner_test

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	return ptyOn(root, exe)
}

// ptyOn returns a fresh pty runner over root, as after a server restart.
func ptyOn(root, exe string) *nativerunner.Pty {
	return &nativerunner.Pty{
		Root:         root,
		Holder:       []string{exe},
		Claude:       exe,
		PollInterval: 10 * time.Millisecond,
		StartTimeout: 10 * time.Second,
		PromptGrace:  500 * time.Millisecond,
	}
}

func TestPty_Conformance(t *testing.T) {
	runnertest.Run(t, func(t *testing.T) runnertest.Harness {
		p := newPty(t)
		return runnertest.Harness{
			Runner: p,
			FinishTurn: func(t *testing.T, s agentrunner.Session) {
				ptyControl(t, s, agentfake.Control{Action: agentfake.ActionFinish})
			},
			Block: func(t *testing.T, s agentrunner.Session, reason string) {
				ptyControl(t, s, agentfake.Control{Action: agentfake.ActionBlock, WaitingFor: reason})
			},
			Stall: func(t *testing.T, s agentrunner.Session) {
				ptyControl(t, s, agentfake.Control{Action: agentfake.ActionStall})
			},
			BackgroundTask: func(t *testing.T, s agentrunner.Session, running bool) {
				ptyControl(t, s, agentfake.Control{Action: agentfake.ActionBackground, Running: running})
			},
			Restart: func(t *testing.T) agentrunner.Runner {
				r := ptyOn(p.Root, p.Claude)
				entries, err := os.ReadDir(p.Root)
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range entries {
					if _, v, err := r.Reattach(e.Name()); err != nil || v != nativerunner.VerdictLive {
						t.Fatalf("Reattach(%s) = %v, %v; want live", e.Name(), v, err)
					}
				}
				return r
			},
			Skip: map[string]string{
				"BlockedThenAnswer": "pty answer lands in ticket 30",
				"AnswerNotBlocked":  "pty answer lands in ticket 30",
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

func TestPty_ClaudeExitIsSessionDiedWithCode(t *testing.T) {
	p := newPty(t)
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(s)
	ptyControl(t, s, agentfake.Control{Action: agentfake.ActionExit, Code: 3})
	_, err = p.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second)
	if !errors.Is(err, agentrunner.ErrSessionDied) || !strings.Contains(err.Error(), "claude exited (code 3)") {
		t.Fatalf("Wait = %v; want session died with code 3", err)
	}
}

func TestPty_HolderCrashIsSessionDied(t *testing.T) {
	p := newPty(t)
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
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
	if err := syscall.Kill(meta.HolderPID, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []struct {
		name string
		call func() error
	}{
		{"Status", func() error { _, err := p.Status(s); return err }},
		{"Prompt", func() error { return p.Prompt(s, "hi") }},
	} {
		deadline := time.Now().Add(5 * time.Second)
		err := verb.call()
		for err == nil && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
			err = verb.call()
		}
		if !errors.Is(err, agentrunner.ErrSessionDied) || !strings.Contains(err.Error(), "holder") {
			t.Errorf("%s = %v; want session died naming the holder", verb.name, err)
		}
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

// An upgrade must never park a live pty run: a holder that predates the
// version field and every optional op is still adopted.
func TestPty_ReattachAdoptsOlderHolder(t *testing.T) {
	p := &nativerunner.Pty{Root: shortTempDir(t), BackgroundTasks: func(agentrunner.Session) bool { return false }}
	dir := filepath.Join(p.Root, "e-01")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	self, ok, err := nativerunner.PS.Lookup(os.Getpid())
	if err != nil || !ok {
		t.Fatalf("lookup self: %v", err)
	}
	meta, err := json.Marshal(nativerunner.Meta{
		Runner: "pty", Label: "e-01", Epic: "e", SessionID: "sid",
		HolderPID: os.Getpid(), HolderPIDStart: self.Start,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nativerunner.MetaFile), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	serveOldHolder(t, filepath.Join(dir, nativerunner.HolderSocketFile), map[string]any{
		"ping":   map[string]any{"pid": os.Getpid()},
		"status": map[string]any{"claude": nativerunner.PidStatus{Status: nativerunner.ClaudeIdle}, "turn": 3},
	})

	s, verdict, err := p.Reattach("e-01")
	if err != nil || verdict != nativerunner.VerdictLive {
		t.Fatalf("Reattach = %v, %v; want live", verdict, err)
	}
	if got, ok, _ := p.Find("e-01"); !ok || got != s {
		t.Errorf("Find = %+v, %v; want %+v", got, ok, s)
	}
	st, err := p.Status(s)
	if err != nil || st.State != agentrunner.StateIdle || st.Turn != 3 || st.SessionID != "sid" {
		t.Errorf("Status = %+v, %v; want idle at turn 3 of sid", st, err)
	}
}

// serveOldHolder answers holder.sock ops from resps and every other op as
// unsupported, like a holder from an older gx.
func serveOldHolder(t *testing.T, sock string, resps map[string]any) {
	t.Helper()
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			var req struct{ Op string }
			if json.NewDecoder(conn).Decode(&req) == nil {
				resp, ok := resps[req.Op]
				if !ok {
					resp = map[string]any{"error": "unsupported op " + req.Op}
				}
				_ = json.NewEncoder(conn).Encode(resp)
			}
			conn.Close()
		}
	}()
}
