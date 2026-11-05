package agentfake

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
)

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
	conv := NewConversationID(t)
	cmd := Harness{}.Launch(harness.LaunchOptions{SessionID: conv})
	p := HoldProgram(t, cmd, conv, ProgramFifoDelayEnv+"=500ms")

	p.report("working", "", "")
	p.WaitStatus(harness.Status{State: harness.Working})
}

func TestHarness_WatchFollowsProgramReportsThroughHolder(t *testing.T) {
	conv := NewConversationID(t)
	cmd := Harness{}.Launch(harness.LaunchOptions{SessionID: conv})
	p := HoldProgram(t, cmd, conv)

	if got := p.Watcher.Status(); got != (harness.Status{State: harness.Starting}) {
		t.Errorf("status before any report = %+v; want Starting", got)
	}
	p.WaitFor("the conversation id", func() bool { return p.Watcher.ConversationID() == conv })

	p.report("idle", "", "")
	p.WaitStatus(harness.Status{State: harness.Idle})
	p.report("working", "", "")
	p.WaitStatus(harness.Status{State: harness.Working})
	p.report("done", "", "")
	p.WaitStatus(harness.Status{State: harness.Idle})
	p.report("working", "", "")
	p.WaitStatus(harness.Status{State: harness.Working})
	p.report("idle", "", "")
	p.WaitStatus(harness.Status{State: harness.Idle, Cancelled: true})
	p.report("blocked", "permission", "Run ls?")
	p.WaitStatus(harness.Status{State: harness.Blocked, Blocked: harness.BlockedPermission, Message: "Run ls?"})
	p.report("blocked", "question", "Red or blue?")
	p.WaitStatus(harness.Status{State: harness.Blocked, Blocked: harness.BlockedQuestion, Message: "Red or blue?"})
}

func TestHarness_ProgramEchoesInputAndSetsTitle(t *testing.T) {
	conv := NewConversationID(t)
	p := HoldProgram(t, Harness{}.Launch(harness.LaunchOptions{SessionID: conv}), conv)

	if err := p.client.SendKeys("hello\r"); err != nil {
		t.Fatal(err)
	}
	p.WaitScreen("hello")
	// The holder's reply to the probe is not typed text.
	if screen := strings.Join(p.rows(), "\n"); strings.Contains(screen, "7501") {
		t.Errorf("screen echoes the probe reply: %q", screen)
	}

	p.control(Control{Action: ActionTitle, Title: "Fix the flaky test"})
	p.WaitFor("the display name", func() bool {
		raw, err := os.ReadFile(filepath.Join(p.Dir, holder.RawLogFile))
		name, ok := Harness{}.DisplayName(raw)
		return err == nil && ok && name == "Fix the flaky test"
	})
}

func TestHarness_ProgramExitsWithCode(t *testing.T) {
	conv := NewConversationID(t)
	p := HoldProgram(t, Harness{}.Launch(harness.LaunchOptions{SessionID: conv}), conv)

	p.control(Control{Action: ActionExit, Code: 3})
	if err := <-p.done; err != nil {
		t.Fatal(err)
	}
	p.done <- nil
	code, err := os.ReadFile(filepath.Join(p.Dir, holder.ExitCodeFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(code)); got != "3" {
		t.Errorf("exit code = %q; want 3", got)
	}
}

func TestHarness_ResumeReportsTheResumedConversation(t *testing.T) {
	conv := NewConversationID(t)
	cmd, ok := Harness{}.Resume(conv, harness.LaunchOptions{})
	if !ok {
		t.Fatal("Resume ok = false")
	}
	p := HoldProgram(t, cmd, conv)
	p.WaitFor("the resumed conversation id", func() bool { return p.Watcher.ConversationID() == conv })
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
	conv := NewConversationID(t)
	p := HoldProgram(t, Harness{}.Launch(harness.LaunchOptions{SessionID: conv}), conv, ProgramTrustEnv+"=1")

	p.WaitScreen(ProgramTrustPrompt)
	startup := Harness{}.StartupScreen(nil, p.rows())
	if startup.Dialog != harness.TrustDialog {
		t.Fatalf("startup = %+v; want the trust dialog", startup)
	}
	if err := p.client.SendKeys(startup.Key); err != nil {
		t.Fatal(err)
	}
	p.WaitFor("the dialog to close", func() bool {
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
