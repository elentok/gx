package agentfake

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/harness"
)

// Fake harness names, registered by RegisterHarness.
const (
	HarnessKind agentrunner.Kind = "agentfake"
	// NoResumeHarnessKind is the fake harness that cannot resume.
	NoResumeHarnessKind agentrunner.Kind = "agentfake-no-resume"
)

// RegisterHarness registers the fake harnesses. Call it from TestMain, so
// the server and holder tests never name claude.
func RegisterHarness() {
	harness.Register(HarnessKind, Harness{})
	harness.Register(NoResumeHarnessKind, Harness{NoResume: true})
}

// Harness runs the fake program: the test binary itself, with ProgramEnv
// set. Its status comes from OSC 7501 reports alone.
type Harness struct {
	NoResume bool
}

func (Harness) Launch(opts harness.LaunchOptions) harness.Command {
	return programCommand("--session-id", opts.SessionID, opts)
}

func (h Harness) Resume(conversationID string, opts harness.LaunchOptions) (harness.Command, bool) {
	if h.NoResume {
		return harness.Command{}, false
	}
	return programCommand("--resume", conversationID, opts), true
}

func programCommand(flag, conversationID string, opts harness.LaunchOptions) harness.Command {
	return harness.Command{
		Name: os.Args[0],
		Args: append([]string{flag, conversationID}, opts.Args...),
		Env:  append(os.Environ(), append([]string{ProgramEnv + "=1"}, opts.Env...)...),
	}
}

func (Harness) Watch(pid int, _ time.Time, _ harness.Screen, _ <-chan struct{}) (harness.Watcher, error) {
	return &programWatcher{pid: pid}, nil
}

func (Harness) StartupScreen(_, rows []string) harness.Startup {
	for _, row := range rows {
		if strings.Contains(row, ProgramTrustPrompt) {
			return harness.Startup{Dialog: harness.TrustDialog, Key: "\r"}
		}
	}
	return harness.Startup{}
}

func (Harness) DisplayName(raw []byte) (string, bool) {
	return screen.Title(raw)
}

type programWatcher struct {
	pid    int
	mu     sync.Mutex
	status harness.Status
}

func (w *programWatcher) Status() harness.Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

// ConversationID rereads the program's file each time, so a program that
// took over a reused pid replaces a stale id.
func (w *programWatcher) ConversationID() string {
	data, err := os.ReadFile(programConversationPath(w.pid))
	if err != nil {
		return ""
	}
	return string(data)
}

// Program ignores clear: the program sends it on exit, and Exited comes from
// the process's death.
func (w *programWatcher) Program(r harness.ProgramReport) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch r.State {
	case "working":
		w.status = harness.Status{State: harness.Working}
	case "done":
		w.status = harness.Status{State: harness.Idle}
	case "idle":
		// idle without done is an Esc-cancel, but only of a turn or a dialog.
		cancelled := w.status.State == harness.Working || w.status.State == harness.Blocked
		w.status = harness.Status{State: harness.Idle, Cancelled: cancelled}
	case "blocked":
		kind := harness.BlockedQuestion
		if r.Kind == string(harness.BlockedPermission) {
			kind = harness.BlockedPermission
		}
		w.status = harness.Status{State: harness.Blocked, Blocked: kind, Message: r.Message}
	}
}
