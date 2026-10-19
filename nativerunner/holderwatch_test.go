package nativerunner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	agentharness "github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// startTestHolder runs a holder in-process and dials it once holder.sock
// answers; finished closes when the holder returns.
func startTestHolder(t *testing.T, dir string, h agentharness.Harness, argv []string) (c *nativerunner.HolderClient, finished chan struct{}) {
	t.Helper()
	finished = make(chan struct{})
	go func() {
		defer close(finished)
		_ = nativerunner.RunTestHolder(dir, h, argv)
	}()
	waitFor(t, "holder.sock answers", func() bool {
		var err error
		c, err = nativerunner.DialHolder(dir)
		return err == nil
	})
	return c, finished
}

func TestHolderWatch_StreamsStatusConversationAndExit(t *testing.T) {
	dir := shortTempDir(t)
	stop := filepath.Join(dir, "stop")
	w := &settableWatcher{}
	child := `printf '\033[1mbold\033[0m\n'; while [ ! -e "$1" ]; do sleep 0.05; done; exit 4`
	c, finished := startTestHolder(t, dir, settableHarness{w: w}, []string{"sh", "-c", child, "sh", stop})
	t.Cleanup(func() {
		_ = c.Stop()
		<-finished
	})
	if !c.Supports(nativerunner.OpWatch) {
		t.Fatal("ping does not list watch")
	}
	watch, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watch.Close() })

	next := func() nativerunner.WatchMessage {
		t.Helper()
		m, err := watch.Next()
		if err != nil {
			t.Fatalf("watch: %v", err)
		}
		return m
	}
	if m := next(); m.Type != nativerunner.StreamStatus || m.Status.State != agentharness.Starting {
		t.Errorf("first message = %+v; want the Starting status", m)
	}
	blocked := agentharness.Status{State: agentharness.Blocked, Blocked: agentharness.BlockedQuestion, Message: "Red or blue?"}
	w.set(blocked, "")
	if m := next(); m.Type != nativerunner.StreamStatus || m.Status != blocked {
		t.Errorf("after a status change = %+v; want %+v", m, blocked)
	}
	w.set(blocked, "conv-1")
	if m := next(); m.Type != nativerunner.StreamStatus || m.ConversationID != "conv-1" || m.Status != blocked {
		t.Errorf("after a conversation id change = %+v; want conv-1", m)
	}

	if err := os.WriteFile(stop, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if m := next(); m.Type != nativerunner.StreamExit || m.ExitCode != 4 {
		t.Errorf("on exit = %+v; want exit code 4", m)
	}
	// The exit is only sent once the last screen is saved.
	last, err := os.ReadFile(filepath.Join(dir, nativerunner.LastScreenFile))
	if err != nil || !strings.Contains(string(last), "bold") || !strings.Contains(string(last), "\x1b[") {
		t.Errorf("last screen = %q, %v; want styled rows", last, err)
	}
	if m, err := watch.Next(); err == nil {
		t.Errorf("after exit got %+v; want the stream closed", m)
	}
}

func TestHolderWatch_IdleStreamDoesNoPeriodicWork(t *testing.T) {
	dir := shortTempDir(t)
	w := &settableWatcher{}
	c, finished := startTestHolder(t, dir, settableHarness{w: w}, []string{"sh", "-c", "sleep 30"})
	t.Cleanup(func() {
		_ = c.Stop()
		<-finished
	})
	watch, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watch.Close() })
	if m, err := watch.Next(); err != nil || m.Type != nativerunner.StreamStatus {
		t.Fatalf("first message = %+v, %v; want a status", m, err)
	}

	before := w.statusReads()
	time.Sleep(500 * time.Millisecond)
	if after := w.statusReads(); after != before {
		t.Errorf("an idle watch read the status %d more times; want none", after-before)
	}

	working := agentharness.Status{State: agentharness.Working}
	w.set(working, "")
	if m, err := watch.Next(); err != nil || m.Status != working {
		t.Errorf("after a change = %+v, %v; want %+v", m, err, working)
	}
}

func TestHolderWatch_ClosesWithoutExitWhenHolderDies(t *testing.T) {
	dir := shortTempDir(t)
	launcher := exec.Command(os.Args[0], "--session-id", "s", "--dir", dir, "--", "sh", "-c", "sleep 30")
	launcher.Env = append(os.Environ(), holderLaunchEnv+"=1")
	out, err := launcher.Output()
	if err != nil {
		t.Fatalf("launcher: %v", err)
	}
	pid, err := strconv.Atoi(string(out))
	if err != nil {
		t.Fatalf("launcher printed %q", out)
	}
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
	var c *nativerunner.HolderClient
	waitFor(t, "holder.sock answers", func() bool {
		c, err = nativerunner.DialHolder(dir)
		return err == nil
	})
	watch, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watch.Close() })
	if m, err := watch.Next(); err != nil || m.Type != nativerunner.StreamStatus {
		t.Fatalf("first message = %+v, %v; want a status", m, err)
	}

	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if m, err := watch.Next(); err == nil {
		t.Errorf("after the holder died got %+v; want the stream closed without exit", m)
	}
}

func TestHolderWatch_OlderHolderDoesNotListIt(t *testing.T) {
	nativerunner.SetHolderOptionalOps(t, nil)
	dir := shortTempDir(t)
	c, finished := startTestHolder(t, dir, settableHarness{w: &settableWatcher{}}, []string{"sh", "-c", "sleep 30"})
	t.Cleanup(func() {
		_ = c.Stop()
		<-finished
	})
	if c.Supports(nativerunner.OpWatch) {
		t.Error("an older holder lists watch")
	}
}

func TestHolder_SetsAgentSessionInChildEnv(t *testing.T) {
	dir := shortTempDir(t)
	child := `printf 'session=%s\n' "$` + nativerunner.AgentSessionEnv + `"`
	if err := nativerunner.RunTestSessionHolder(dir, settableHarness{w: &settableWatcher{}}, "gx-42", []string{"sh", "-c", child}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, nativerunner.RawLogFile))
	if !strings.Contains(string(raw), "session=gx-42") {
		t.Errorf("raw.log = %q; want the child to see the gx session id", raw)
	}
}

// settableHarness is a harness whose Watch reports what the test sets.
type settableHarness struct {
	agentharness.Harness
	w *settableWatcher
}

func (h settableHarness) Watch(int, time.Time, agentharness.Screen, <-chan struct{}) (agentharness.Watcher, error) {
	return h.w, nil
}

func (settableHarness) DisplayName([]byte) (string, bool) { return "", false }

type settableWatcher struct {
	mu             sync.Mutex
	status         agentharness.Status
	conversationID string
	onChange       func()
	ready          agentharness.ReadySignal
	// reads counts Status calls.
	reads int
}

func (w *settableWatcher) set(s agentharness.Status, conversationID string) {
	w.mu.Lock()
	w.status, w.conversationID = s, conversationID
	onChange := w.onChange
	w.mu.Unlock()
	if onChange != nil {
		onChange()
	}
}

func (w *settableWatcher) OnChange(f func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onChange = f
}

func (w *settableWatcher) Status() agentharness.Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.reads++
	return w.status
}

func (w *settableWatcher) statusReads() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.reads
}

func (w *settableWatcher) ConversationID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conversationID
}

func (w *settableWatcher) Program(agentharness.ProgramReport) {}

func (w *settableWatcher) Ready() agentharness.ReadySignal {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ready
}

func (w *settableWatcher) setReady(s agentharness.ReadySignal) {
	w.mu.Lock()
	w.ready = s
	onChange := w.onChange
	w.mu.Unlock()
	if onChange != nil {
		onChange()
	}
}

func TestHolderWatch_ReportsReadySignal(t *testing.T) {
	dir := shortTempDir(t)
	w := &settableWatcher{}
	c, finished := startTestHolder(t, dir, settableHarness{w: w}, []string{"sh", "-c", "sleep 30"})
	t.Cleanup(func() {
		_ = c.Stop()
		<-finished
	})
	watch, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watch.Close() })

	if m, err := watch.Next(); err != nil || m.Type != nativerunner.StreamStatus || m.Ready != "" {
		t.Fatalf("first message = %+v, %v; want a status that is not ready", m, err)
	}
	w.setReady(agentharness.ReadyScreen)
	if m, err := watch.Next(); err != nil || m.Type != nativerunner.StreamStatus || m.Ready != agentharness.ReadyScreen {
		t.Errorf("after ready = %+v, %v; want ready by %q", m, err, agentharness.ReadyScreen)
	}
}
