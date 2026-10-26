package agentfake

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
)

// The test binary doubles as the TUI fake, so tests drive it as the PTY
// runner would: a separate process with its own pid file.
func TestMain(m *testing.M) {
	if os.Getenv(TUIEnv) == "1" {
		TUIClaude()
	}
	if os.Getenv(ProgramEnv) == "1" {
		Program()
	}
	RegisterHarness()
	os.Exit(m.Run())
}

var tuiLaunchArgs = []string{
	"--session-id", "sess-1",
	"--permission-mode", "auto",
	"--settings", `{"tui":"default"}`,
	"--disallowedTools", "AskUserQuestion",
}

type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type tuiFake struct {
	t      *testing.T
	home   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	screen *syncBuffer
	done   chan struct{}
}

func startTUIFake(t *testing.T, args []string, env ...string) *tuiFake {
	t.Helper()
	f := &tuiFake{t: t, home: t.TempDir(), screen: &syncBuffer{}, done: make(chan struct{})}
	f.cmd = exec.Command(os.Args[0], args...)
	f.cmd.Dir = t.TempDir()
	f.cmd.Env = append(os.Environ(), append(env, TUIEnv+"=1", "HOME="+f.home)...)
	f.cmd.Stdout = f.screen
	f.cmd.Stderr = f.screen
	stdin, err := f.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	f.stdin = stdin
	if err := f.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = f.cmd.Wait()
		close(f.done)
	}()
	t.Cleanup(func() {
		_ = f.cmd.Process.Kill()
		<-f.done
	})
	return f
}

func (f *tuiFake) control(c Control) {
	f.t.Helper()
	if err := SendTUIControl(f.home, "sess-1", c); err != nil {
		f.t.Fatal(err)
	}
}

func (f *tuiFake) typeLine(s string) {
	f.t.Helper()
	if _, err := io.WriteString(f.stdin, s+"\r"); err != nil {
		f.t.Fatal(err)
	}
}

// pidFile reads the raw pid file, or nil while it is absent.
func (f *tuiFake) pidFile() map[string]any {
	data, err := os.ReadFile(PidFilePath(f.home, f.cmd.Process.Pid))
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		f.t.Fatalf("pid file %q: %v", data, err)
	}
	return m
}

func (f *tuiFake) waitFor(what string, cond func() bool) {
	f.t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			f.t.Fatalf("timed out waiting for %s; screen: %q; pid file: %v", what, f.screen.String(), f.pidFile())
		}
	}
}

func (f *tuiFake) waitStatus(status, waitingFor string) {
	f.t.Helper()
	f.waitFor("status "+status+" "+waitingFor, func() bool {
		m := f.pidFile()
		w, hasWaitingFor := m["waitingFor"]
		return m != nil && m["status"] == status && (waitingFor == "" && !hasWaitingFor || w == waitingFor)
	})
}

func (f *tuiFake) waitScreen(text string) {
	f.t.Helper()
	f.waitFor("screen text "+text, func() bool { return strings.Contains(f.screen.String(), text) })
}

func (f *tuiFake) waitExit() int {
	f.t.Helper()
	select {
	case <-f.done:
		return f.cmd.ProcessState.ExitCode()
	case <-time.After(5 * time.Second):
		f.t.Fatal("fake did not exit")
		return 0
	}
}

func TestTUIClaude_PidFileFollowsControls(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")
	m := f.pidFile()
	for _, field := range []string{"pid", "sessionId", "cwd", "startedAt", "procStart", "version", "kind", "entrypoint", "updatedAt", "statusUpdatedAt"} {
		if _, ok := m[field]; !ok {
			t.Errorf("pid file lacks %q: %v", field, m)
		}
	}
	if m["sessionId"] != "sess-1" || m["pid"] != float64(f.cmd.Process.Pid) || m["kind"] != "interactive" {
		t.Errorf("pid file = %v", m)
	}

	f.typeLine("hello")
	f.waitStatus(StatusBusy, "")
	f.waitScreen("> hello")

	f.control(Control{Action: ActionBlock, WaitingFor: "plan approval"})
	f.waitStatus(StatusWaiting, "plan approval")
	f.waitScreen("Waiting for plan approval")

	f.typeLine("")
	f.waitStatus(StatusBusy, "")

	f.control(Control{Action: ActionBlock})
	f.waitStatus(StatusWaiting, "permission prompt")

	f.control(Control{Action: ActionFinish})
	f.waitStatus(StatusIdle, "")

	probe := claudeui.BackgroundTasks(f.home, time.Now)
	session := agentrunner.Session{Label: "a", SessionID: "sess-1"}
	f.control(Control{Action: ActionBackground, Running: true})
	f.waitFor("a running background task", func() bool { return probe(session) })
	f.control(Control{Action: ActionBackground})
	f.waitFor("the background task to end", func() bool { return !probe(session) })

	f.control(Control{Action: ActionExit})
	if code := f.waitExit(); code != 0 {
		t.Fatalf("exit code = %d, want 0; screen: %q", code, f.screen.String())
	}
	if _, err := os.Stat(PidFilePath(f.home, f.cmd.Process.Pid)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pid file after clean exit: %v", err)
	}
}

func TestTUIClaude_EmitsOSCAndChangesSessionID(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")

	f.control(Control{Action: ActionReport, State: "working"})
	f.waitFor("the OSC report", func() bool { return strings.Contains(f.screen.String(), "\x1b]7501;state=working") })

	f.control(Control{Action: ActionConversation, ConversationID: "sess-2"})
	f.waitFor("the new sessionId", func() bool { return f.pidFile()["sessionId"] == "sess-2" })
}

func TestTUIClaude_StallLeavesPromptsUnstarted(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")
	f.control(Control{Action: ActionStall})
	// The stall and the prompt travel on different channels, so give the
	// stall a moment to land first.
	time.Sleep(100 * time.Millisecond)
	f.typeLine("hello")
	time.Sleep(200 * time.Millisecond)
	if m := f.pidFile(); m["status"] != StatusIdle {
		t.Fatalf("status after a stalled prompt = %v, want idle", m["status"])
	}
}

func TestTUIClaude_CtrlCEndsTheTurnAndRestoresThePrompt(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")
	f.typeLine("hello")
	f.waitStatus(StatusBusy, "")
	// A lone Esc leaves a running turn alone.
	if _, err := io.WriteString(f.stdin, "\x1b"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if m := f.pidFile(); m["status"] != StatusBusy {
		t.Fatalf("status after Esc = %v, want busy", m["status"])
	}
	if _, err := io.WriteString(f.stdin, "\x03"); err != nil {
		t.Fatal(err)
	}
	f.waitStatus(StatusIdle, "")
	f.waitScreen("> hello")
	if _, err := io.WriteString(f.stdin, "\x15"); err != nil {
		t.Fatal(err)
	}
	f.typeLine("again")
	f.waitScreen("> again")
}

func TestTUIClaude_ExitControlExitsWithCode(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")
	f.control(Control{Action: ActionExit, Code: 3})
	if code := f.waitExit(); code != 3 {
		t.Fatalf("exit code = %d, want 3; screen: %q", code, f.screen.String())
	}
	if f.pidFile() == nil {
		t.Fatal("a crash exit removed the pid file")
	}
}

func TestTUIClaude_FailingWorkerExitsNonZero(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")
	cfg := `{"behaviour":{"p:e/01":"` + BehaviourFail + `"}}`
	if err := os.WriteFile(filepath.Join(f.cmd.Dir, WorkerConfigFile), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	f.typeLine("/gx-implement p:e/01")
	if code := f.waitExit(); code != 1 {
		t.Fatalf("exit code = %d, want 1; screen: %q", code, f.screen.String())
	}
}

func TestTUIClaude_TrustDialogBeforePidFile(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs, TUITrustEnv+"=1")
	f.waitFor("the trust dialog", func() bool {
		return claudeui.ClassifyStartup(claudeui.Normalize(f.screen.String())) == claudeui.StartupTrust
	})
	if f.pidFile() != nil {
		t.Fatal("pid file written before the trust dialog was answered")
	}
	f.typeLine("\x1b[B")
	f.waitStatus(StatusIdle, "")
}

func TestTUIClaude_TrustDialogEnterOnNoExits(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs, TUITrustEnv+"=1")
	f.waitScreen("❯ No, exit")
	f.typeLine("")
	if code := f.waitExit(); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestTUIClaude_NoPidFile(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs, TUINoPidFileEnv+"=1")
	f.waitScreen("Welcome to Claude Code")
	f.control(Control{Action: ActionExit, Code: 1})
	f.waitExit()
	entries, _ := os.ReadDir(filepath.Dir(PidFilePath(f.home, 0)))
	if len(entries) != 0 {
		t.Fatalf("sessions dir = %v, want no pid file", entries)
	}
}

func TestParseTUIArgs_AcceptsRemoteControl(t *testing.T) {
	args := append([]string{"--remote-control", "fix-agent"}, tuiLaunchArgs...)
	if _, err := parseTUIArgs(args); err != nil {
		t.Fatalf("parseTUIArgs: %v", err)
	}
}

func TestClassify_InvestigatePromptIsASlashPrompt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, WorkerConfigFile), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tu := &tui{pid: PidFile{Cwd: dir}}
	kind, _, _ := tu.classify("/gx-investigate demo:paused-epic/07 is needs-repair: Run failed")
	if kind != promptSlash {
		t.Fatalf("kind = %v, want promptSlash", kind)
	}
	kind, _, _ = tu.classify("/gx-implement demo:paused-epic/07")
	if kind != promptWorker {
		t.Fatalf("control: kind = %v, want promptWorker", kind)
	}
}

func TestTUIClaude_RejectsUnknownFlag(t *testing.T) {
	f := startTUIFake(t, append([]string{"--bogus"}, tuiLaunchArgs...))
	if code := f.waitExit(); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}
