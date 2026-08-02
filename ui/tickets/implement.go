package tickets

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
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
	done     int
	total    int
	sink     *ralphloop.ChannelEventSink
	// lastErrEpic/lastErr carry the most recently finished run's error (nil on
	// success) past finish() clearing running/epicName, so handleImplementPoll
	// and handleImplementSync — which only learn a run ended once running flips
	// back to false — can still tell success from failure and surface it,
	// instead of ralphloop.Run's error being silently discarded (as it
	// previously was at the cmdStartImplement call site: a ticket left
	// Status: claimed by a failed launch, e.g. an iteration-branch name
	// collision on AddWorktree, used to get reported as a plain "finished"
	// toast with no indication anything went wrong).
	lastErrEpic string
	lastErr     error
}

var ralphLoopRegistry = &loopRegistry{}

// tryStart claims the registry for a new run against epicName, returning the
// sink to feed ralphloop.Run and false->true ok if one is already in flight.
// done/total seed the cross-tab overlay's (ticket 06) progress count from the
// epic's on-disk state at launch time, since the live-event stream itself
// never reports a ticket total — only the Model's live-event drain
// (modelLiveEventMsg, model.go) advances done as tickets land afterward.
func (r *loopRegistry) tryStart(epicName string, done, total int) (*ralphloop.ChannelEventSink, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil, false
	}
	r.running = true
	r.epicName = epicName
	r.done = done
	r.total = total
	r.sink = ralphloop.NewChannelEventSink()
	return r.sink, true
}

func (r *loopRegistry) finish(epicName string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	r.epicName = ""
	r.done = 0
	r.total = 0
	r.sink = nil
	r.lastErrEpic = epicName
	r.lastErr = err
}

// takeLastError reports (and clears) the error the most recent run against
// epicName finished with, so a poll/sync handler only surfaces it once.
func (r *loopRegistry) takeLastError(epicName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastErrEpic != epicName {
		return nil
	}
	err := r.lastErr
	r.lastErrEpic = ""
	r.lastErr = nil
	return err
}

// recordTicketFinished advances the progress count by one landed ticket; see
// model.go's modelLiveEventMsg handling, which calls this on every
// LiveEventIterationFinished as it drains the registry's live-event channel.
// Guarded by running so a stray call after finish() can't resurrect a stale
// count.
func (r *loopRegistry) recordTicketFinished() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		r.done++
	}
}

// LoopStatus reports ralphLoopRegistry's current state for the app shell's
// cross-tab status overlay (ticket 06), which polls it the same way this
// package's own OnPageActivated/handleImplementPoll do rather than
// subscribing directly, since the app shell only routes tea.Msgs to the
// active page.
func LoopStatus() (running bool, epicName string, done, total int) {
	ralphLoopRegistry.mu.Lock()
	defer ralphLoopRegistry.mu.Unlock()
	return ralphLoopRegistry.running, ralphLoopRegistry.epicName, ralphLoopRegistry.done, ralphLoopRegistry.total
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

// IsLoopRunning reports whether a ralph-loop launched from this process is
// currently in flight, regardless of which worktree it targets. The app
// shell (ticket 05) uses this to force the tickets tab into --all scope
// while a loop is running, since the loop keeps going against its own
// worktree even after the user navigates the worktree cursor elsewhere.
func IsLoopRunning() bool {
	return ralphLoopRegistry.isRunning()
}

// snapshot reports the currently-running epic and its live event stream, if
// any — events is nil when no run is in flight.
func (r *loopRegistry) snapshot() (running bool, epicName string, events <-chan ralphloop.LiveEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sink != nil {
		events = r.sink.Events()
	}
	return r.running, r.epicName, events
}

// implementStartedMsg reports that a ralph-loop launch was accepted and its
// background goroutine is now running.
type implementStartedMsg struct {
	epicName string
	events   <-chan ralphloop.LiveEvent
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
	events   <-chan ralphloop.LiveEvent
}

// implementFailedMsg reports that a launch never made it to a background
// goroutine at all (already running, or couldn't resolve the repo).
type implementFailedMsg struct {
	err error
}

func newImplementAgentMenu() components.MenuState {
	return components.MenuState{
		Items: []components.MenuItem{
			{Label: "l  Claude", Value: string(ralphloop.AgentClaude)},
			{Label: "o  Codex", Value: string(ralphloop.AgentCodex)},
		},
		Cursor: 0,
	}
}

// bindingTicketsImplement (below in model_keys.go's manager) triggers
// handleImplementKey, gated to epic rows only: pressing "i" on a ticket row
// or with nothing selected is a no-op. Claude is selected by default.
func (m Model) handleImplementKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok || !r.isEpic() {
		return m, nil
	}
	if ralphLoopRegistry.isRunning() {
		return m, notify.Info("a ralph-loop is already running")
	}
	m.implementAgentMenu = newImplementAgentMenu()
	m.implementAgentMenuOpen = true
	return m, nil
}

func (m Model) handleImplementAgentMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "l":
		return m.openImplementConfirm(ralphloop.AgentClaude)
	case "o":
		return m.openImplementConfirm(ralphloop.AgentCodex)
	}

	next, decided, accepted, handled := components.UpdateMenu(msg, m.implementAgentMenu)
	if !handled {
		return m, nil
	}
	m.implementAgentMenu = next
	if !decided {
		return m, nil
	}
	if !accepted {
		m.implementAgentMenuOpen = false
		return m, nil
	}
	agent := ralphloop.AgentKind(m.implementAgentMenu.Items[m.implementAgentMenu.Cursor].Value)
	return m.openImplementConfirm(agent)
}

func (m Model) openImplementConfirm(agent ralphloop.AgentKind) (tea.Model, tea.Cmd) {
	m.implementAgentMenuOpen = false
	r, ok := m.selectedRow()
	if !ok || !r.isEpic() {
		return m, nil
	}
	epic := m.epics[r.epicIdx]
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    fmt.Sprintf("Start implementing epic %q with %s?", epic.Name, agentDisplayName(agent)),
		AcceptCmd: m.cmdStartImplement(epic.Name, agent, epic.DoneCount(), epic.TotalCount()),
	})
	return m, nil
}

func (m Model) implementAgentMenuView() string {
	prompt := "Choose the agent for this ralph-loop:"
	if r, ok := m.selectedRow(); ok && r.isEpic() {
		prompt = fmt.Sprintf("Choose the agent for epic %q:", m.epics[r.epicIdx].Name)
	}
	return components.RenderMenuModal(
		"Implement Epic",
		prompt,
		m.implementAgentMenu,
		"",
		ui.ColorBorder,
		ui.ColorBlue,
		ui.ColorSubtle,
		ui.ColorText,
		48,
	)
}

func agentDisplayName(agent ralphloop.AgentKind) string {
	if agent == ralphloop.AgentCodex {
		return "Codex"
	}
	return "Claude"
}

func (m Model) handleConfirmUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.Update(msg)
	m.confirm = next
	return m, cmd
}

func (m Model) handleImplementStarted(msg implementStartedMsg) (tea.Model, tea.Cmd) {
	m.implementEpic = msg.epicName
	liveCmd := m.startLiveTracking(msg.events)
	return m, tea.Batch(m.implementSpinner.Tick, cmdPollImplement(msg.epicName), liveCmd)
}

// handleImplementPoll re-checks ralphLoopRegistry; once epicName's run has
// left the registry, it's done (ralphloop.Run always calls registry.finish()
// before returning, success or failure alike) — takeLastError distinguishes
// the two so a launch failure (e.g. a ticket left stuck Status: claimed) is
// reported instead of a misleadingly plain "finished".
func (m Model) handleImplementPoll(msg implementPollMsg) (tea.Model, tea.Cmd) {
	if running, epicName, _ := ralphLoopRegistry.snapshot(); running && epicName == msg.epicName {
		return m, cmdPollImplement(msg.epicName)
	}
	m.clearLiveTracking()
	return m, tea.Batch(implementFinishedNotifyCmd(msg.epicName), m.cmdLoad())
}

// implementFinishedNotifyCmd reports epicName's just-finished run: an error
// toast if ralphloop.Run returned one, otherwise the plain completion toast.
func implementFinishedNotifyCmd(epicName string) tea.Cmd {
	if err := ralphLoopRegistry.takeLastError(epicName); err != nil {
		return notify.Error(fmt.Sprintf("ralph-loop failed for epic %q: %v", epicName, err))
	}
	return notify.Info(fmt.Sprintf("ralph-loop finished for epic %q", epicName))
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
		liveCmd := m.startLiveTracking(msg.events)
		return m, tea.Batch(m.implementSpinner.Tick, cmdPollImplement(msg.epicName), liveCmd)
	}
	if m.implementEpic == "" {
		return m, nil
	}
	finished := m.implementEpic
	m.clearLiveTracking()
	return m, tea.Batch(implementFinishedNotifyCmd(finished), m.cmdLoad())
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
		running, epicName, events := ralphLoopRegistry.snapshot()
		return implementSyncMsg{running: running, epicName: epicName, events: events}
	}
}

// cmdStartImplement claims ralphLoopRegistry and launches ralphloop.Run as a
// background goroutine against epicName, porting the RunOptions assembly
// cmd/ralphloop.go's runRalphLoop uses for the headless `gx ralph-loop`
// command (agent/skill/maxParallel/smartZone defaults, ScratchDir resolved
// to an absolute path for the same reason runRalphLoop does). registry.sink
// is the run's only event consumer route: the launched Model drains it via
// implementStartedMsg.events (model.go's modelLiveEventMsg handling), which
// both renders live state (ticket 02) and calls recordTicketFinished (ticket
// 06) — a second, independent drain here would race that one for events off
// the same channel.
func (m Model) cmdStartImplement(epicName string, agent ralphloop.AgentKind, done, total int) tea.Cmd {
	worktreeRoot := m.worktreeRoot
	return func() tea.Msg {
		sink, ok := ralphLoopRegistry.tryStart(epicName, done, total)
		if !ok {
			return implementFailedMsg{err: fmt.Errorf("a ralph-loop is already running")}
		}
		opts, err := buildImplementRunOptions(worktreeRoot, epicName, agent)
		if err != nil {
			ralphLoopRegistry.finish(epicName, nil)
			return implementFailedMsg{err: err}
		}
		go func() {
			err := ralphloop.Run(opts, ralphloop.DefaultDeps(), sink)
			ralphLoopRegistry.finish(epicName, err)
		}()
		return implementStartedMsg{epicName: epicName, events: sink.Events()}
	}
}

// cmdPollImplement re-checks ralphLoopRegistry after implementPollInterval;
// see handleImplementPoll.
func cmdPollImplement(epicName string) tea.Cmd {
	return tea.Tick(implementPollInterval, func(time.Time) tea.Msg {
		return implementPollMsg{epicName: epicName}
	})
}

func buildImplementRunOptions(worktreeRoot, epicName string, agent ralphloop.AgentKind) (ralphloop.RunOptions, error) {
	repo, err := git.FindRepo(worktreeRoot)
	if err != nil {
		return ralphloop.RunOptions{}, err
	}
	return ralphloop.RunOptions{
		EpicName:    epicName,
		Agent:       agent,
		Skill:       "implement",
		RepoDir:     repo.Root,
		ScratchDir:  filepath.Join(worktreeRoot, ".scratch"),
		MaxParallel: 2,
		SmartZone:   150_000,
	}, nil
}
