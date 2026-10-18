package nativerunner_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
			// The reason is dropped: the suite answers allow, which only a
			// permission prompt takes.
			Block: func(t *testing.T, s agentrunner.Session, _ string) {
				ptyControl(t, s, agentfake.Control{Action: agentfake.ActionBlock, WaitingFor: "permission prompt"})
			},
			BlockOnDialog: func(t *testing.T, s agentrunner.Session, permission bool) {
				waitingFor := "input needed"
				if permission {
					waitingFor = "permission prompt"
				}
				ptyControl(t, s, agentfake.Control{Action: agentfake.ActionBlock, WaitingFor: waitingFor})
			},
			LastPrompt: func(t *testing.T, s agentrunner.Session) string {
				return ptyLastPrompt(t, p, s)
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

// ptyLastPrompt reads the prompt off the TUI fake's working screen.
func ptyLastPrompt(t *testing.T, p *nativerunner.Pty, s agentrunner.Session) string {
	t.Helper()
	c, err := nativerunner.DialHolder(filepath.Join(p.Root, s.Label))
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := c.Screen(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if prompt, ok := strings.CutPrefix(row, "> "); ok {
			return strings.TrimSpace(prompt)
		}
	}
	return ""
}

// blockedPty starts a pty agent and leaves it on a permission prompt; a
// stalled one keeps the prompt open on Esc.
func blockedPty(t *testing.T, p *nativerunner.Pty, stalled bool) agentrunner.Session {
	t.Helper()
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop(s) })
	// Stall goes first: controls arrive in order, so the blocked wait below
	// also confirms the stall landed before any Esc.
	if stalled {
		ptyControl(t, s, agentfake.Control{Action: agentfake.ActionStall})
	}
	ptyControl(t, s, agentfake.Control{Action: agentfake.ActionBlock, WaitingFor: "permission prompt"})
	if _, err := p.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPty_DenyReasonFallbackOnOlderHolder(t *testing.T) {
	p := newPty(t)
	t.Setenv(holderNoOpsEnv, "1")
	s := blockedPty(t, p, false)

	if err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny, Text: "use rg"}); err != nil {
		t.Fatal(err)
	}
	if st, err := p.Status(s); err != nil || st.State != agentrunner.StateWorking {
		t.Fatalf("Status = %+v, %v; want working on the reason", st, err)
	}
	if got := ptyLastPrompt(t, p, s); got != "use rg" {
		t.Fatalf("last prompt = %q, want the reason", got)
	}
}

// A deny that never reaches idle leaves the session died, so the loop's next
// Wait parks the ticket needs-repair.
func TestPty_DenyReasonIdleTimeoutIsSessionDied(t *testing.T) {
	for _, tc := range []struct {
		name  string
		noOps string
	}{
		{"holder op", ""},
		{"fallback", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPty(t)
			t.Setenv(holderDenyTimeoutEnv, "200ms")
			t.Setenv(holderNoOpsEnv, tc.noOps)
			s := blockedPty(t, p, true)

			err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny, Text: "use rg"})
			if !errors.Is(err, agentrunner.ErrSessionDied) {
				t.Fatalf("Answer = %v; want ErrSessionDied", err)
			}
			if _, err := p.Wait(s, []agentrunner.State{agentrunner.StateIdle}, time.Second); !errors.Is(err, agentrunner.ErrSessionDied) {
				t.Fatalf("Wait after a failed deny = %v; want ErrSessionDied", err)
			}
		})
	}
}

func TestPty_StartAnswersTrustDialog(t *testing.T) {
	for _, tc := range []struct {
		name    string
		dropKey string
	}{
		{"first key read", ""},
		{"first key dropped", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPty(t)
			t.Setenv(agentfake.TUITrustEnv, "1")
			t.Setenv(agentfake.TUITrustDropKeyEnv, tc.dropKey)
			s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
			if err != nil {
				t.Fatal(err)
			}
			defer p.Stop(s)
			if st, err := p.Status(s); err != nil || st.State != agentrunner.StateIdle {
				t.Errorf("Status = %+v, %v; want idle", st, err)
			}
		})
	}
}

func TestPty_StartFailsFastWhenClaudeExits(t *testing.T) {
	p := newPty(t)
	p.Claude = "false"
	began := time.Now()
	_, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err == nil || errors.Is(err, agentrunner.ErrMissingCapability) || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("Start = %v; want a claude or holder exit, not a missing pid file", err)
	}
	if elapsed := time.Since(began); elapsed > p.StartTimeout/2 {
		t.Errorf("Start took %v; want it to fail before the start deadline", elapsed)
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

func TestPty_StuckPromptNudgesAndRetypesOnce(t *testing.T) {
	p := newPty(t)
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(s)
	ptyControl(t, s, agentfake.Control{Action: agentfake.ActionStall})

	if err := p.Prompt(s, "do it"); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("Prompt = %v; want ErrNotDelivered", err)
	}
	in, err := os.ReadFile(agentfake.TUIInputPath(os.Getenv("HOME"), s.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	if want := "do it\r" + "\r" + "\x15do it\r"; string(in) != want {
		t.Errorf("keys typed = %q; want submit, one nudge, one cleared retype: %q", in, want)
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

// The holder's status reports the exit while it drains, before exit-code is
// on disk.
func TestPty_ExitBeforeExitCodeFileIsClaudeExited(t *testing.T) {
	p := &nativerunner.Pty{Root: t.TempDir()}
	err := nativerunner.Died(p, "e-01", fmt.Errorf("wait: %w", &nativerunner.ClaudeExitedError{Code: 3}))
	if !errors.Is(err, agentrunner.ErrSessionDied) || !strings.HasSuffix(err.Error(), ": claude exited (code 3)") || strings.Contains(err.Error(), "holder failed") {
		t.Fatalf("died = %v; want session died: claude exited (code 3)", err)
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
	p, dir := preparePtyAgent(t, ptyAgentProcs())
	serveOldHolder(t, dir, map[string]any{
		"ping":   map[string]any{"pid": agentPID},
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
	if err != nil || st.State != agentrunner.StateIdle || st.Turn != 3 || st.SessionID != agentSID {
		t.Errorf("Status = %+v, %v; want idle at turn 3 of %s", st, err, agentSID)
	}
}

// gatedProcs holds its first lookup until release closes.
type gatedProcs struct {
	*fakeProcs
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (g *gatedProcs) Lookup(pid int) (nativerunner.Process, bool, error) {
	g.once.Do(func() {
		close(g.entered)
		<-g.release
	})
	return g.fakeProcs.Lookup(pid)
}

// Reattach's ps lookup and holder dial can be slow, so they must not block
// calls for the runner's other sessions.
func TestPty_ReattachDoesNotBlockOtherCalls(t *testing.T) {
	procs := &gatedProcs{fakeProcs: ptyAgentProcs(), entered: make(chan struct{}), release: make(chan struct{})}
	p, dir := preparePtyAgent(t, procs.fakeProcs)
	p.Procs = procs
	serveOldHolder(t, dir, map[string]any{"ping": map[string]any{"pid": agentPID}})

	done := make(chan error, 1)
	go func() {
		_, verdict, err := p.Reattach("e-01")
		if err == nil && verdict != nativerunner.VerdictLive {
			err = fmt.Errorf("verdict %v; want live", verdict)
		}
		done <- err
	}()
	<-procs.entered

	found := make(chan struct{})
	go func() {
		_, _, _ = p.Find("e-02")
		close(found)
	}()
	select {
	case <-found:
	case <-time.After(2 * time.Second):
		t.Error("Find blocked while Reattach was in progress")
	}
	close(procs.release)
	if err := <-done; err != nil {
		t.Fatalf("Reattach = %v", err)
	}
	if _, ok, _ := p.Find("e-01"); !ok {
		t.Error("Reattach did not adopt e-01")
	}
}

// Interactive claude stays alive after its turn, so any pty agent found dead
// died, even one whose last turn ended.
func TestPty_ReattachDeadAgentDied(t *testing.T) {
	reused := nativerunner.Process{Start: "Tue Oct  7 09:00:00 2026", Cmdline: "claude --session-id " + agentSID}
	tests := []struct {
		name   string
		procs  func(*fakeProcs)
		silent bool
	}{
		{name: "holder gone", procs: func(f *fakeProcs) { f.kill(holderPID) }},
		{name: "claude gone", procs: func(f *fakeProcs) { f.kill(agentPID) }},
		{name: "holder pid reused", procs: func(f *fakeProcs) { f.procs[holderPID] = reused }},
		{name: "claude pid reused", procs: func(f *fakeProcs) { f.procs[agentPID] = reused }},
		{name: "holder lacks session id", procs: func(f *fakeProcs) {
			f.procs[holderPID] = nativerunner.Process{Start: holderStart, Cmdline: "vim notes.md"}
		}},
		{name: "socket silent", silent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			procs := ptyAgentProcs()
			if tt.procs != nil {
				tt.procs(procs)
			}
			p, dir := preparePtyAgent(t, procs)
			if err := os.WriteFile(filepath.Join(dir, nativerunner.ExitCodeFile), []byte("0\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if tt.silent {
				serveOldHolder(t, dir, nil)
			} else {
				serveOldHolder(t, dir, map[string]any{"ping": map[string]any{"pid": agentPID}})
			}

			_, verdict, err := p.Reattach("e-01")
			if err != nil || verdict != nativerunner.VerdictDied {
				t.Fatalf("Reattach = %v, %v; want died", verdict, err)
			}
			if _, found, _ := p.Find("e-01"); found {
				t.Error("a dead agent was adopted")
			}
		})
	}
}

const (
	holderPID   = agentPID + 1
	holderStart = "Mon Oct  6 09:59:59 2026"
)

// ptyAgentProcs is a process table where the holder and claude both still run.
func ptyAgentProcs() *fakeProcs {
	procs := agentProcs()
	procs.procs[holderPID] = nativerunner.Process{
		Start:   holderStart,
		Cmdline: "gx agent-holder --session-id " + agentSID + " -- claude --session-id " + agentSID,
	}
	return procs
}

// preparePtyAgent writes a pty agent directory as a previous server left it
// and returns a fresh runner over it.
func preparePtyAgent(t *testing.T, procs *fakeProcs) (*nativerunner.Pty, string) {
	t.Helper()
	p := &nativerunner.Pty{
		Root:            shortTempDir(t),
		Procs:           procs,
		BackgroundTasks: func(agentrunner.Session) bool { return false },
	}
	dir := filepath.Join(p.Root, "e-01")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, err := json.Marshal(nativerunner.Meta{
		Runner: "pty", Label: "e-01", Epic: "e", SessionID: agentSID,
		PID: agentPID, PIDStart: agentStart, HolderPID: holderPID, HolderPIDStart: holderStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nativerunner.MetaFile), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	return p, dir
}

// serveOldHolder answers dir's holder.sock ops from resps and every other op
// as unsupported, like a holder from an older gx. With nil resps it hangs up
// without answering, like a stuck holder.
func serveOldHolder(t *testing.T, dir string, resps map[string]any) {
	t.Helper()
	ln, err := net.Listen("unix", filepath.Join(dir, nativerunner.HolderSocketFile))
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
			if json.NewDecoder(conn).Decode(&req) == nil && resps != nil {
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
