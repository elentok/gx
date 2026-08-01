package tickets

import (
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
)

// implementPollInterval is how often a tickets.Model polls ralphLoopRegistry
// for completion. Polling (rather than a done-channel the launching Model
// alone waits on) is what lets *any* tickets.Model instance — including one
// rebuilt by a tab switch away and back, see OnPageActivated — pick up an
// in-flight run's state: the app shell only routes tea.Msgs to the active
// page, so a message sent to a backgrounded page's model is silently
// dropped, and the done-channel-blocking Cmd would rebind to a fresh model
// value each time OnPageActivated fires without an independent way to check
// "is it still running" first.
const implementPollInterval = 300 * time.Millisecond

// loopRegistry enforces "only one ralph-loop may run at a time per gx
// process" (ticket 01's acceptance criterion), which has to hold across
// every tickets.Model instance in the process — not just the one that
// launched it, since a worktree-context switch rebuilds the Model but the
// background loop keeps running. A package-level singleton, guarded by a
// mutex since the launching goroutine and the finishing goroutine both touch
// it, is the simplest thing that's still correct if this process ever hosts
// more than one tickets.Model (e.g. --all mode).
type loopRegistry struct {
	mu       sync.Mutex
	running  bool
	epicName string
}

var ralphLoopRegistry = &loopRegistry{}

// tryStart claims the registry for a new run against epicName, returning
// false if one is already in flight.
func (r *loopRegistry) tryStart(epicName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return false
	}
	r.running = true
	r.epicName = epicName
	return true
}

func (r *loopRegistry) finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	r.epicName = ""
}

// CanQuit implements the app shell's quit-guard duck type (see
// ui/app/model_quit.go): it blocks quitting gx while this process has a
// ralph-loop in flight, since an interrupted loop can leave the worktree
// mid-cherry-pick. This is warn-then-allow, not a hard block — reconcile.go
// recovers an interrupted loop on the next run, so there's no correctness
// reason to prevent quitting outright.
func (m Model) CanQuit() bool {
	return !ralphLoopRegistry.isRunning()
}

func (r *loopRegistry) isRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// snapshot reports the currently-running epic, if any.
func (r *loopRegistry) snapshot() (running bool, epicName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running, r.epicName
}

// implementStartedMsg reports that a ralph-loop launch was accepted and its
// background goroutine is now running.
type implementStartedMsg struct {
	epicName string
}

// implementPollMsg drives the poll loop started by implementStartedMsg/
// OnPageActivated: on each tick it re-checks ralphLoopRegistry for epicName.
type implementPollMsg struct {
	epicName string
}

// implementSyncMsg reports ralphLoopRegistry's state as observed when this
// tab (re)gained focus (see OnPageActivated), so a Model that missed the
// completion message while another tab was active can catch up.
type implementSyncMsg struct {
	running  bool
	epicName string
}

// implementFailedMsg reports that a launch never made it to a background
// goroutine at all (already running, or couldn't resolve the repo).
type implementFailedMsg struct {
	err error
}

// bindingTicketsImplement (below in model_keys.go's manager) triggers
// handleImplementKey, gated to epic rows only: pressing "i" on a ticket row
// or with nothing selected is a no-op.
func (m Model) handleImplementKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok || !r.isEpic() {
		return m, nil
	}
	if ralphLoopRegistry.isRunning() {
		return m, notify.Info("a ralph-loop is already running")
	}
	epic := m.epics[r.epicIdx]
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    fmt.Sprintf("Start implementing epic %q?", epic.Name),
		AcceptCmd: m.cmdStartImplement(epic.Name),
	})
	return m, nil
}

func (m Model) handleConfirmUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.Update(msg)
	m.confirm = next
	return m, cmd
}

func (m Model) handleImplementStarted(msg implementStartedMsg) (tea.Model, tea.Cmd) {
	m.implementEpic = msg.epicName
	return m, tea.Batch(m.implementSpinner.Tick, cmdPollImplement(msg.epicName))
}

// handleImplementPoll re-checks ralphLoopRegistry; once epicName's run has
// left the registry, it's done (ralphloop.Run always calls registry.finish()
// before returning, success or failure alike — ticket 01 only needs "it's
// done", not pass/fail detail).
func (m Model) handleImplementPoll(msg implementPollMsg) (tea.Model, tea.Cmd) {
	if running, epicName := ralphLoopRegistry.snapshot(); running && epicName == msg.epicName {
		return m, cmdPollImplement(msg.epicName)
	}
	m.implementEpic = ""
	return m, tea.Batch(notify.Info(fmt.Sprintf("ralph-loop finished for epic %q", msg.epicName)), m.cmdLoad())
}

// handleImplementSync answers OnPageActivated's resync Cmd: it reconciles
// this Model's implementEpic against ralphLoopRegistry's live state, which
// may have changed while this tab was in the background (a plain tea.Msg
// sent to a backgrounded page is dropped by the app shell, so the model that
// launched a run can't rely on ever seeing its own completion message if the
// user switched away in the meantime).
func (m Model) handleImplementSync(msg implementSyncMsg) (tea.Model, tea.Cmd) {
	if msg.running {
		m.implementEpic = msg.epicName
		return m, tea.Batch(m.implementSpinner.Tick, cmdPollImplement(msg.epicName))
	}
	if m.implementEpic == "" {
		return m, nil
	}
	finished := m.implementEpic
	m.implementEpic = ""
	return m, tea.Batch(notify.Info(fmt.Sprintf("ralph-loop finished for epic %q", finished)), m.cmdLoad())
}

func (m Model) handleImplementSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if m.implementEpic == "" {
		return m, nil
	}
	var cmd tea.Cmd
	m.implementSpinner, cmd = m.implementSpinner.Update(msg)
	return m, cmd
}

// OnPageActivated implements the app shell's pageActivationAware duck-type
// (see ui/app/model_tabs.go), firing every time this tab (re)gains focus —
// including the very first time. It fires an implementSyncMsg to catch this
// Model up with ralphLoopRegistry, since it may have missed a completion
// message that arrived while another tab was active.
func (m Model) OnPageActivated() tea.Cmd {
	return func() tea.Msg {
		running, epicName := ralphLoopRegistry.snapshot()
		return implementSyncMsg{running: running, epicName: epicName}
	}
}

// cmdStartImplement claims ralphLoopRegistry and launches ralphloop.Run as a
// background goroutine against epicName, porting the RunOptions assembly
// cmd/ralphloop.go's runRalphLoop uses for the headless `gx ralph-loop`
// command (agent/skill/maxParallel/smartZone defaults, ScratchDir resolved
// to an absolute path for the same reason runRalphLoop does). The launch's
// own transcript/lifecycle detail isn't rendered yet (ticket 02's job) — a
// discarding text sink is enough to satisfy the EventSink contract here.
func (m Model) cmdStartImplement(epicName string) tea.Cmd {
	worktreeRoot := m.worktreeRoot
	return func() tea.Msg {
		if !ralphLoopRegistry.tryStart(epicName) {
			return implementFailedMsg{err: fmt.Errorf("a ralph-loop is already running")}
		}
		opts, err := buildImplementRunOptions(worktreeRoot, epicName)
		if err != nil {
			ralphLoopRegistry.finish()
			return implementFailedMsg{err: err}
		}
		go func() {
			defer ralphLoopRegistry.finish()
			_ = ralphloop.Run(opts, ralphloop.DefaultDeps(), ralphloop.NewTextEventSink(io.Discard))
		}()
		return implementStartedMsg{epicName: epicName}
	}
}

// cmdPollImplement re-checks ralphLoopRegistry after implementPollInterval;
// see handleImplementPoll.
func cmdPollImplement(epicName string) tea.Cmd {
	return tea.Tick(implementPollInterval, func(time.Time) tea.Msg {
		return implementPollMsg{epicName: epicName}
	})
}

func buildImplementRunOptions(worktreeRoot, epicName string) (ralphloop.RunOptions, error) {
	repo, err := git.FindRepo(worktreeRoot)
	if err != nil {
		return ralphloop.RunOptions{}, err
	}
	return ralphloop.RunOptions{
		EpicName:    epicName,
		Agent:       ralphloop.AgentClaude,
		Skill:       "implement",
		RepoDir:     repo.Root,
		ScratchDir:  filepath.Join(worktreeRoot, ".scratch"),
		MaxParallel: 2,
		SmartZone:   150_000,
	}, nil
}
