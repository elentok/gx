package agentfake

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// capturingKind runs the fake harness and hands the test the holder's
// watcher.
const capturingKind agentrunner.Kind = "agentfake-capturing"

type capturingHarness struct {
	Harness
	watchers chan harness.Watcher
}

func (h capturingHarness) Watch(pid int, spawned time.Time, scr harness.Screen, stop <-chan struct{}) (harness.Watcher, error) {
	w, err := h.Harness.Watch(pid, spawned, scr, stop)
	h.watchers <- w
	return w, err
}

type heldProgram struct {
	t       *testing.T
	dir     string
	conv    string
	client  *nativerunner.HolderClient
	watcher harness.Watcher
	done    chan error
}

// holdProgram runs cmd in a real holder. The holder starts its child from argv
// alone, so env reaches it through HolderOptions.Env rather than cmd.Env.
func holdProgram(t *testing.T, cmd harness.Command, conv string, env ...string) *heldProgram {
	t.Helper()
	progDir := NewProgramDir(t)
	env = append([]string{ProgramEnv + "=1", ProgramDirEnv + "=" + progDir}, env...)
	watchers := make(chan harness.Watcher, 1)
	// The holder's watch runs in this process, so it gets the dir directly.
	harness.Register(capturingKind, capturingHarness{Harness: Harness{Dir: progDir}, watchers: watchers})
	dir, err := os.MkdirTemp("", "gxh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	p := &heldProgram{t: t, dir: dir, conv: conv, done: make(chan error, 1)}
	go func() {
		p.done <- nativerunner.RunHarnessHolder(dir, capturingKind, nativerunner.HolderOptions{Env: env}, append([]string{cmd.Name}, cmd.Args...))
	}()
	select {
	case p.watcher = <-watchers:
	case err := <-p.done:
		t.Fatalf("holder exited early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("holder never started a watch")
	}
	p.waitFor("holder.sock answers", func() bool {
		p.client, err = nativerunner.DialHolder(dir)
		return err == nil
	})
	t.Cleanup(func() {
		_ = p.client.Stop()
		<-p.done
	})
	return p
}

func (p *heldProgram) waitFor(what string, cond func() bool) {
	p.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			p.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func (p *heldProgram) control(c Control) {
	p.t.Helper()
	if err := SendProgramControl(p.conv, c); err != nil {
		p.t.Fatal(err)
	}
}

func (p *heldProgram) report(state, kind, msg string) {
	p.t.Helper()
	p.control(Control{Action: ActionReport, State: state, Kind: kind, Message: msg})
}

func (p *heldProgram) waitStatus(want harness.Status) {
	p.t.Helper()
	p.waitFor(fmt.Sprintf("status %+v", want), func() bool { return p.watcher.Status() == want })
}

func (p *heldProgram) rows() []string {
	rows, _, err := p.client.Screen(0)
	if err != nil {
		p.t.Fatal(err)
	}
	return rows
}

func (p *heldProgram) waitScreen(text string) {
	p.t.Helper()
	p.waitFor("the screen shows "+text, func() bool { return strings.Contains(strings.Join(p.rows(), "\n"), text) })
}

func TestProgramDir_IsPerTestAndRemovedAfterIt(t *testing.T) {
	var a, b string
	t.Run("a", func(t *testing.T) {
		a = NewProgramDir(t)
		b = NewProgramDir(t)
		if a == b {
			t.Fatalf("two harnesses share %s", a)
		}
	})
	for _, dir := range []string{a, b} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s outlived its test: %v", dir, err)
		}
	}
}

func TestSendProgramControl_WaitsUntilTheProgramOpenedItsFIFO(t *testing.T) {
	old := fifoOpenWait
	fifoOpenWait = 50 * time.Millisecond
	t.Cleanup(func() { fifoOpenWait = old })
	conv := newConversationID(t)
	cmd := Harness{}.Launch(harness.LaunchOptions{SessionID: conv})
	p := holdProgram(t, cmd, conv, ProgramFifoDelayEnv+"=500ms")

	p.report("working", "", "")
	p.waitStatus(harness.Status{State: harness.Working})
}

func newConversationID(t *testing.T) string {
	return fmt.Sprintf("%s-%d", strings.ReplaceAll(t.Name(), "/", "-"), time.Now().UnixNano())
}

func TestHarness_WatchFollowsProgramReportsThroughHolder(t *testing.T) {
	conv := newConversationID(t)
	cmd := Harness{}.Launch(harness.LaunchOptions{SessionID: conv})
	p := holdProgram(t, cmd, conv)

	if got := p.watcher.Status(); got != (harness.Status{State: harness.Starting}) {
		t.Errorf("status before any report = %+v; want Starting", got)
	}
	p.waitFor("the conversation id", func() bool { return p.watcher.ConversationID() == conv })

	p.report("idle", "", "")
	p.waitStatus(harness.Status{State: harness.Idle})
	p.report("working", "", "")
	p.waitStatus(harness.Status{State: harness.Working})
	p.report("done", "", "")
	p.waitStatus(harness.Status{State: harness.Idle})
	p.report("working", "", "")
	p.waitStatus(harness.Status{State: harness.Working})
	p.report("idle", "", "")
	p.waitStatus(harness.Status{State: harness.Idle, Cancelled: true})
	p.report("blocked", "permission", "Run ls?")
	p.waitStatus(harness.Status{State: harness.Blocked, Blocked: harness.BlockedPermission, Message: "Run ls?"})
	p.report("blocked", "question", "Red or blue?")
	p.waitStatus(harness.Status{State: harness.Blocked, Blocked: harness.BlockedQuestion, Message: "Red or blue?"})
}

func TestHarness_ProgramEchoesInputAndSetsTitle(t *testing.T) {
	conv := newConversationID(t)
	p := holdProgram(t, Harness{}.Launch(harness.LaunchOptions{SessionID: conv}), conv)

	if err := p.client.SendKeys("hello\r"); err != nil {
		t.Fatal(err)
	}
	p.waitScreen("hello")
	// The holder's reply to the probe is not typed text.
	if screen := strings.Join(p.rows(), "\n"); strings.Contains(screen, "7501") {
		t.Errorf("screen echoes the probe reply: %q", screen)
	}

	p.control(Control{Action: ActionTitle, Title: "Fix the flaky test"})
	p.waitFor("the display name", func() bool {
		raw, err := os.ReadFile(filepath.Join(p.dir, nativerunner.RawLogFile))
		name, ok := Harness{}.DisplayName(raw)
		return err == nil && ok && name == "Fix the flaky test"
	})
}

func TestHarness_ProgramExitsWithCode(t *testing.T) {
	conv := newConversationID(t)
	p := holdProgram(t, Harness{}.Launch(harness.LaunchOptions{SessionID: conv}), conv)

	p.control(Control{Action: ActionExit, Code: 3})
	if err := <-p.done; err != nil {
		t.Fatal(err)
	}
	p.done <- nil
	code, err := os.ReadFile(filepath.Join(p.dir, nativerunner.ExitCodeFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(code)); got != "3" {
		t.Errorf("exit code = %q; want 3", got)
	}
}

func TestHarness_ResumeReportsTheResumedConversation(t *testing.T) {
	conv := newConversationID(t)
	cmd, ok := Harness{}.Resume(conv, harness.LaunchOptions{})
	if !ok {
		t.Fatal("Resume ok = false")
	}
	p := holdProgram(t, cmd, conv)
	p.waitFor("the resumed conversation id", func() bool { return p.watcher.ConversationID() == conv })
}

func TestHarness_ResumeCanBeTurnedOff(t *testing.T) {
	h, err := harness.Lookup(NoResumeHarnessKind)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.Resume("c1", harness.LaunchOptions{}); ok {
		t.Error("Resume ok = true; want false")
	}
}

func TestHarness_StartupScreenAnswersTrustDialog(t *testing.T) {
	conv := newConversationID(t)
	p := holdProgram(t, Harness{}.Launch(harness.LaunchOptions{SessionID: conv}), conv, ProgramTrustEnv+"=1")

	p.waitScreen(ProgramTrustPrompt)
	startup := Harness{}.StartupScreen(nil, p.rows())
	if startup.Dialog != harness.TrustDialog {
		t.Fatalf("startup = %+v; want the trust dialog", startup)
	}
	if err := p.client.SendKeys(startup.Key); err != nil {
		t.Fatal(err)
	}
	p.waitFor("the dialog to close", func() bool {
		return Harness{}.StartupScreen(nil, p.rows()).Dialog == harness.NoDialog
	})
}

func TestProgramWatcher_ReadyOnFirstIdleReport(t *testing.T) {
	w := &programWatcher{startup: func() bool { return false }}
	if got := w.Ready(); got != "" {
		t.Fatalf("before any report: Ready = %q, want none", got)
	}
	w.Program(harness.ProgramReport{State: "blocked", Kind: "question", Progress: -1})
	w.Program(harness.ProgramReport{State: "done", Progress: -1})
	w.Program(harness.ProgramReport{State: "working", Progress: -1})
	if got := w.Ready(); got != harness.ReadyOSC7501 {
		t.Fatalf("after idle: Ready = %q, want %q", got, harness.ReadyOSC7501)
	}
	w.Program(harness.ProgramReport{State: "blocked", Kind: "question", Progress: -1})
	if got := w.Ready(); got != "" {
		t.Fatalf("while Blocked: Ready = %q, want none", got)
	}
}
