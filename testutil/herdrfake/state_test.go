package herdrfake

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// capturingTB wraps a real *testing.T but captures Logf output instead of
// forwarding it, so watchdog dump assertions can inspect what would have
// been logged without depending on the real test's own log output.
type capturingTB struct {
	*testing.T
	mu   sync.Mutex
	logs []string
}

func (c *capturingTB) Logf(format string, args ...any) {
	c.mu.Lock()
	c.logs = append(c.logs, fmt.Sprintf(format, args...))
	c.mu.Unlock()
}

func (c *capturingTB) allLogs() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.logs, "\n")
}

func registerCounter(s *State) *int {
	count := 0
	s.Register("counter", "inc", func(s *State, argv []string) (any, Identities, error) {
		count++
		return map[string]int{"count": count}, Identities{}, nil
	})
	return &count
}

// TestState_SerializesConcurrentHelperRequests dispatches the same command
// concurrently from many real helper processes (through the process
// boundary, not by calling State.dispatch directly) and checks the shared
// counter lands on exactly the expected total — proof mutations are
// serialized rather than racing. Run with -race to also catch any data race
// in State itself.
func TestState_SerializesConcurrentHelperRequests(t *testing.T) {
	s := NewState(t)
	count := registerCounter(s)
	StartState(t, s)

	const n = 20
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			if out, err := exec.Command("herdr", "counter", "inc").CombinedOutput(); err != nil {
				t.Errorf("herdr counter inc: %v\n%s", err, out)
			}
		})
	}
	wg.Wait()

	if *count != n {
		t.Errorf("count = %d, want %d", *count, n)
	}
	if got := len(s.Trace()); got != n {
		t.Errorf("len(Trace()) = %d, want %d", got, n)
	}
}

// TestState_TraceRecordsSequenceArgvVirtualTimeIdentitiesAndSnapshots covers
// every field a trace entry should carry: monotonically increasing
// sequence numbers, the exact argv dispatched, State's virtual time at
// dispatch, the identities the command reports touching, before/after state
// snapshots that differ once the command mutates state, and the response.
func TestState_TraceRecordsSequenceArgvVirtualTimeIdentitiesAndSnapshots(t *testing.T) {
	s := NewState(t)
	s.Register("workspace", "create", func(s *State, argv []string) (any, Identities, error) {
		ws := &Workspace{ID: "ws-1", Label: argv[2]}
		s.Workspaces[ws.ID] = ws
		return map[string]string{"workspace_id": ws.ID}, Identities{WorkspaceID: ws.ID}, nil
	})
	StartState(t, s)

	s.AdvanceVirtualTime(3 * time.Second)

	if out, err := exec.Command("herdr", "workspace", "create", "mylabel").CombinedOutput(); err != nil {
		t.Fatalf("herdr workspace create: %v\n%s", err, out)
	}

	trace := s.Trace()
	if len(trace) != 1 {
		t.Fatalf("len(Trace()) = %d, want 1", len(trace))
	}
	e := trace[0]

	if e.Seq != 1 {
		t.Errorf("Seq = %d, want 1", e.Seq)
	}
	wantArgv := []string{"workspace", "create", "mylabel"}
	if strings.Join(e.Argv, ",") != strings.Join(wantArgv, ",") {
		t.Errorf("Argv = %v, want %v", e.Argv, wantArgv)
	}
	if e.VirtualTime != 3*time.Second {
		t.Errorf("VirtualTime = %s, want 3s", e.VirtualTime)
	}
	if e.Identities.WorkspaceID != "ws-1" {
		t.Errorf("Identities.WorkspaceID = %q, want %q", e.Identities.WorkspaceID, "ws-1")
	}
	if strings.Contains(e.Before, "ws-1") {
		t.Errorf("Before snapshot already contains ws-1: %s", e.Before)
	}
	if !strings.Contains(e.After, "ws-1") {
		t.Errorf("After snapshot missing ws-1: %s", e.After)
	}
	if !strings.Contains(string(e.Output), "ws-1") {
		t.Errorf("Output missing ws-1: %s", e.Output)
	}

	// A second dispatch gets the next sequence number.
	if _, err := exec.Command("herdr", "workspace", "create", "other").CombinedOutput(); err != nil {
		t.Fatalf("herdr workspace create: %v", err)
	}
	trace = s.Trace()
	if len(trace) != 2 || trace[1].Seq != 2 {
		t.Fatalf("second entry Seq = %+v, want Seq 2", trace)
	}
}

// TestState_UnimplementedCommandFails checks that a command with no
// registered CommandFunc fails instead of returning a zero-value default —
// unregistered "tab list" must not succeed with an empty tab list.
func TestState_UnimplementedCommandFails(t *testing.T) {
	s := NewState(t)
	StartState(t, s)

	out, err := exec.Command("herdr", "tab", "list").CombinedOutput()
	if err == nil {
		t.Fatalf("herdr tab list: want non-zero exit, got success with output %s", out)
	}
	if !strings.Contains(string(out), "unimplemented command: tab list") {
		t.Errorf("output = %q, want it to mention the unimplemented command", out)
	}
}

// TestState_ShortArgvFails checks that a command line too short to contain
// a verb+noun also fails fast rather than panicking or defaulting.
func TestState_ShortArgvFails(t *testing.T) {
	s := NewState(t)
	StartState(t, s)

	out, err := exec.Command("herdr", "workspace").CombinedOutput()
	if err == nil {
		t.Fatalf("herdr workspace: want non-zero exit, got success with output %s", out)
	}
	if !strings.Contains(string(out), "command too short") {
		t.Errorf("output = %q, want it to mention the command is too short", out)
	}
}

// TestState_WatchdogDumpsTraceOnDeadlock blocks a command handler well past
// watchdogDeadlockBudget and checks the watchdog dumps the trace collected
// so far without waiting for the blocked command to return — proving trace
// reads don't route through the same mutex a stuck command holds.
func TestState_WatchdogDumpsTraceOnDeadlock(t *testing.T) {
	ctb := &capturingTB{T: t}
	s := NewState(ctb)
	registerCounter(s)

	block := make(chan struct{})
	s.Register("stuck", "cmd", func(s *State, argv []string) (any, Identities, error) {
		<-block
		return nil, Identities{}, nil
	})

	stop := make(chan struct{})
	defer close(stop)
	go s.runWatchdog(stop)

	// One prior, completed dispatch so the dumped trace is non-empty beyond
	// the stuck entry.
	s.dispatch([]string{"counter", "inc"})

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.dispatch([]string{"stuck", "cmd"})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(ctb.allLogs(), "protocol deadlock") {
		if time.Now().After(deadline) {
			t.Fatalf("watchdog never dumped a protocol deadlock; logs so far:\n%s", ctb.allLogs())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(ctb.allLogs(), "[counter inc]") {
		t.Errorf("dump missing the prior completed command; logs:\n%s", ctb.allLogs())
	}

	close(block)
	<-done
}

// TestState_WatchdogDumpsTraceOnAssertionFailure checks the watchdog also
// dumps the trace once the test has already failed an assertion, even
// without any command hanging. It builds State directly (bypassing NewState)
// with a synthetic failed() so it can simulate an assertion failure without
// actually failing this test.
func TestState_WatchdogDumpsTraceOnAssertionFailure(t *testing.T) {
	var mu sync.Mutex
	var logs []string

	var failed atomic.Bool
	s := &State{
		Workspaces: map[string]*Workspace{},
		Tabs:       map[string]*Tab{},
		Panes:      map[string]*Pane{},
		Agents:     map[string]*Agent{},
		Sessions:   map[string]*Session{},
		commands:   map[string]CommandFunc{},
		logf: func(format string, args ...any) {
			mu.Lock()
			logs = append(logs, fmt.Sprintf(format, args...))
			mu.Unlock()
		},
		failed: failed.Load,
	}
	registerCounter(s)
	s.dispatch([]string{"counter", "inc"})

	stop := make(chan struct{})
	defer close(stop)
	go s.runWatchdog(stop)

	failed.Store(true)

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		got := strings.Join(logs, "\n")
		mu.Unlock()
		if strings.Contains(got, "assertion failure") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("watchdog never dumped on assertion failure; logs so far:\n%s", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
