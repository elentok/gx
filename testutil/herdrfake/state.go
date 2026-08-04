package herdrfake

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// watchdogInterval is how often the watchdog polls for a hung command or a
// failed assertion. watchdogDeadlockBudget is how long a single command may
// run (in real time) before the watchdog treats it as a protocol deadlock.
// Both are deliberately short: the watchdog exists to catch a scenario that
// will never make progress, not to bound legitimate work, which advances
// State's virtual clock instead of sleeping in real time.
const (
	watchdogInterval       = 20 * time.Millisecond
	watchdogDeadlockBudget = 500 * time.Millisecond
)

// Workspace, Tab, Pane, Agent, and Session are the entities State tracks.
// They're deliberately minimal — just enough identity and status for
// command implementations built on State to model herdr's real behavior.
type Workspace struct {
	ID    string
	Label string
	Cwd   string
}

type Tab struct {
	ID          string
	WorkspaceID string
	Label       string
	Number      int
	Focused     bool
}

type Pane struct {
	ID    string
	TabID string
}

type Agent struct {
	ID        string
	PaneID    string
	Name      string
	Kind      string
	Status    string
	SessionID string
}

type Session struct {
	ID      string
	AgentID string
}

// Identities names the workspace/tab/pane/agent/session ids a dispatched
// command touched, for its trace entry. Command implementations set
// whichever fields apply; the rest stay empty.
type Identities struct {
	WorkspaceID string
	TabID       string
	PaneID      string
	AgentID     string
	SessionID   string
}

// CommandFunc implements one herdr subcommand (e.g. "workspace list")
// against shared State. Dispatch calls it with State's mutex held, so it may
// read and mutate State's maps directly without its own locking — but for
// the same reason it must not block waiting for a future state change (e.g.
// a virtual-time advance or a status transition): that would deadlock every
// other concurrent command, including the one that would unblock it.
//
// A non-nil error produces a herdr command failure (CommandError); a nil
// error produces a herdr success envelope wrapping result (Result).
type CommandFunc func(s *State, argv []string) (result any, ids Identities, err error)

// TraceEntry records one dispatched command: its request, State's virtual
// time when it ran, the identities it touched, a snapshot of State
// immediately before and after it ran, and its response.
type TraceEntry struct {
	Seq         uint64
	Argv        []string
	VirtualTime time.Duration
	Identities  Identities
	Before      string
	After       string
	Output      []byte
	ExitCode    int
}

// State is a deterministic, in-memory model of a herdr server: the
// workspace/tab/pane/agent/session graph a coordinated set of fake herdr
// invocations observe and mutate, plus a virtual clock the test advances
// explicitly instead of sleeping in real time.
//
// All command dispatch is serialized under a single mutex, so concurrent
// helper requests (from concurrent goroutines under test, each backed by its
// own re-invoked helper process) observe State's mutations atomically and in
// a well-defined order. Every dispatch gets a monotonically increasing
// sequence number and a trace entry; commands without a registered
// CommandFunc fail instead of returning a zero-value default.
type State struct {
	mu sync.Mutex

	seq         uint64
	virtualTime time.Duration

	Workspaces map[string]*Workspace
	Tabs       map[string]*Tab
	Panes      map[string]*Pane
	Agents     map[string]*Agent
	Sessions   map[string]*Session

	commands map[string]CommandFunc

	// traceMu guards trace independently of mu, so the watchdog can read the
	// trace accumulated so far even while mu is held by a command that never
	// returns — the exact protocol-deadlock case it exists to diagnose.
	traceMu sync.Mutex
	trace   []TraceEntry

	logf     func(format string, args ...any)
	failed   func() bool
	inFlight atomic.Bool
	lastBeat atomic.Int64
	dumped   atomic.Bool
}

// NewState creates an empty State that reports failures and dumps its trace
// through t.
func NewState(t testing.TB) *State {
	return &State{
		Workspaces: map[string]*Workspace{},
		Tabs:       map[string]*Tab{},
		Panes:      map[string]*Pane{},
		Agents:     map[string]*Agent{},
		Sessions:   map[string]*Session{},
		commands:   map[string]CommandFunc{},
		logf:       t.Logf,
		failed:     t.Failed,
	}
}

// Register installs fn as the implementation of the herdr subcommand named
// by verb and noun (e.g. Register("workspace", "list", ...)). Dispatching a
// command with no registered implementation fails fast instead of guessing
// at a default response.
func (s *State) Register(verb, noun string, fn CommandFunc) {
	s.commands[verb+" "+noun] = fn
}

// AdvanceVirtualTime moves State's virtual clock forward by d. Nothing in
// State advances it on its own; command implementations that model
// timeouts, waits, or delays read the current value through VirtualTime and
// rely on the test to call AdvanceVirtualTime explicitly, so a scenario
// never has to sleep in real time.
func (s *State) AdvanceVirtualTime(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.virtualTime += d
}

// VirtualTime returns State's current virtual clock value.
func (s *State) VirtualTime() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.virtualTime
}

// Trace returns a copy of every command dispatched so far, in sequence
// order. It doesn't wait for mu, so it stays readable even while a command
// is stuck holding it.
func (s *State) Trace() []TraceEntry {
	s.traceMu.Lock()
	defer s.traceMu.Unlock()
	return append([]TraceEntry(nil), s.trace...)
}

// Handler returns a herdrfake.Handler backed by State's registered commands,
// suitable for passing to herdrfake.Start.
func (s *State) Handler() Handler {
	return s.dispatch
}

func (s *State) dispatch(argv []string) ([]byte, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.inFlight.Store(true)
	s.beat()
	defer func() {
		s.inFlight.Store(false)
		s.beat()
	}()

	s.seq++
	seq := s.seq
	before := s.snapshot()

	var (
		output   []byte
		exitCode int
		ids      Identities
	)
	if len(argv) < 2 {
		output, exitCode = CommandError(fmt.Sprintf("herdrfake: command too short: %v", argv))
	} else if fn, ok := s.commands[argv[0]+" "+argv[1]]; !ok {
		output, exitCode = CommandError(fmt.Sprintf("herdrfake: unimplemented command: %s %s", argv[0], argv[1]))
	} else {
		result, gotIDs, err := fn(s, argv)
		ids = gotIDs
		if err != nil {
			output, exitCode = CommandError(err.Error())
		} else {
			output, exitCode = Result(result)
		}
	}

	entry := TraceEntry{
		Seq:         seq,
		Argv:        append([]string(nil), argv...),
		VirtualTime: s.virtualTime,
		Identities:  ids,
		Before:      before,
		After:       s.snapshot(),
		Output:      append([]byte(nil), output...),
		ExitCode:    exitCode,
	}
	s.traceMu.Lock()
	s.trace = append(s.trace, entry)
	s.traceMu.Unlock()

	return output, exitCode
}

// snapshot formats State's entities for a trace entry. Must be called with
// s.mu held.
func (s *State) snapshot() string {
	var b strings.Builder
	fmt.Fprintf(&b, "workspaces=%s tabs=%s panes=%s agents=%s sessions=%s virtualTime=%s",
		formatEntities(s.Workspaces), formatEntities(s.Tabs), formatEntities(s.Panes),
		formatEntities(s.Agents), formatEntities(s.Sessions), s.virtualTime)
	return b.String()
}

func formatEntities[T any](m map[string]*T) string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%+v", *m[id]))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (s *State) beat() {
	s.lastBeat.Store(time.Now().UnixNano())
}

// StartState wires State up like Start: it puts a fake herdr executable
// first in PATH for the duration of the test, backed by State's registered
// commands, and starts a short real-time watchdog alongside it. If a single
// command doesn't complete within watchdogDeadlockBudget, or the test has
// already failed an assertion, the watchdog dumps State's full trace via
// t.Log so a hung or failed scenario is diagnosable without waiting out its
// own timeout.
func StartState(t *testing.T, s *State) *Coordinator {
	t.Helper()

	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go s.runWatchdog(stop)

	return Start(t, s.Handler())
}

func (s *State) runWatchdog(stop <-chan struct{}) {
	ticker := time.NewTicker(watchdogInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if s.dumped.Load() {
				continue
			}
			if s.failed() {
				s.dumpTrace("assertion failure")
				continue
			}
			if s.inFlight.Load() && time.Since(time.Unix(0, s.lastBeat.Load())) > watchdogDeadlockBudget {
				s.dumpTrace("protocol deadlock")
			}
		}
	}
}

func (s *State) dumpTrace(reason string) {
	s.dumped.Store(true)
	trace := s.Trace()
	s.logf("herdrfake: watchdog dump (%s), %d trace entries:", reason, len(trace))
	for _, e := range trace {
		s.logf("  #%d argv=%v virtualTime=%s identities=%+v exitCode=%d output=%s\n    before: %s\n    after:  %s",
			e.Seq, e.Argv, e.VirtualTime, e.Identities, e.ExitCode, e.Output, e.Before, e.After)
	}
}
