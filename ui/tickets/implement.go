package tickets

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/nav"
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

// epicRun is one epic's in-flight ralph-loop state.
type epicRun struct {
	drainDone   chan struct{}
	done, total int
	sink        *ralphloop.ChannelEventSink
	gate        *ralphloop.Gate
	state       RunState
	finalError  string
	tickets     map[string]RunTicketSnapshot
	startedAt   time.Time
}

type RunState string

const (
	RunStateRunning   RunState = "running"
	RunStateCompleted RunState = "completed"
	RunStateFailed    RunState = "failed"
)

type RunTicketSnapshot struct {
	Identifier    string
	Label         string
	Running       bool
	Paused        bool
	Completed     bool
	PauseKind     ralphloop.PauseKind
	PauseReason   string
	ContextTokens int
}

type RunSnapshot struct {
	EpicName      string
	State         RunState
	Done          int
	Total         int
	ContextTokens int
	Paused        bool
	FinalError    string
	Tickets       map[string]RunTicketSnapshot
	StartedAt     time.Time
}

// loopRegistry enforces "an epic may not have two ralph-loops running at
// once, and at most maxConcurrent epics may run at once" per gx process
// (ticket 01's original single-epic version of this, capacity-lifted by
// ticket 03), which has to hold across every tickets.Model instance in the
// process — not just the one that launched a given run, since a
// worktree-context switch rebuilds the Model but the background loop keeps
// running. A package-level singleton, guarded by a mutex since the launching
// goroutine and the finishing goroutine both touch it, is the simplest thing
// that's still correct if this process ever hosts more than one
// tickets.Model (e.g. --all mode).
type loopRegistry struct {
	mu            sync.Mutex
	maxConcurrent int
	runs          map[string]*epicRun
	snapshots     map[string]*epicRun
	paused        bool
	// Errors survive run removal so every observer sees the failure until an
	// explicit acknowledgement clears it.
	lastErr map[string]error
}

func newLoopRegistry(maxConcurrent int) *loopRegistry {
	return &loopRegistry{
		maxConcurrent: maxConcurrent,
		runs:          map[string]*epicRun{},
		snapshots:     map[string]*epicRun{},
		lastErr:       map[string]error{},
	}
}

var ralphLoopRegistry = newLoopRegistry(ui.Settings{}.MaxConcurrentEpics())

// ConfigureMaxConcurrentEpics updates the process-wide execution queue cap.
// Existing runs are left alone; a lower cap only delays subsequent starts.
func ConfigureMaxConcurrentEpics(maxConcurrent int) {
	ralphLoopRegistry.setMaxConcurrent(maxConcurrent)
}

func (r *loopRegistry) setMaxConcurrent(maxConcurrent int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.maxConcurrent = max(maxConcurrent, 1)
}

var runRalphLoop = ralphloop.Run

// tryStart claims an epic slot and starts the stream drain before returning
// the producer sink. Snapshots, rather than the channel, are shared with views.
func (r *loopRegistry) tryStart(epicName string, done, total int) (*ralphloop.ChannelEventSink, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.paused {
		return nil, false
	}
	if _, exists := r.runs[epicName]; exists {
		return nil, false
	}
	if len(r.runs) >= r.maxConcurrent {
		return nil, false
	}
	sink := ralphloop.NewChannelEventSink()
	run := &epicRun{
		drainDone: make(chan struct{}),
		done:      done, total: total, sink: sink, gate: ralphloop.NewGate(),
		state: RunStateRunning, tickets: map[string]RunTicketSnapshot{},
		startedAt: time.Now(),
	}
	r.runs[epicName] = run
	r.snapshots[epicName] = run
	go func() {
		defer close(run.drainDone)
		for event := range sink.Events() {
			r.reduceLiveEvent(epicName, event)
		}
	}()
	return sink, true
}

func (r *loopRegistry) reduceLiveEvent(epicName string, event ralphloop.LiveEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.snapshots[epicName]
	if run == nil {
		return
	}
	switch event.Kind {
	case ralphloop.LiveEventIterationStarted, ralphloop.LiveEventTicketReattached:
		run.tickets[event.Identifier] = RunTicketSnapshot{
			Identifier: event.Identifier,
			Label:      event.Label,
			Running:    true,
		}
	case ralphloop.LiveEventContextOccupancy:
		ticket, ok := run.tickets[event.Identifier]
		if ok {
			ticket.ContextTokens = event.Tokens
			run.tickets[event.Identifier] = ticket
		}
	case ralphloop.LiveEventIterationPaused:
		for identifier, ticket := range run.tickets {
			if ticket.Label != event.Label {
				continue
			}
			ticket.Running = false
			ticket.Paused = true
			ticket.PauseKind = event.PauseKind
			ticket.PauseReason = event.Reason
			run.tickets[identifier] = ticket
			break
		}
	case ralphloop.LiveEventIterationResumed:
		for identifier, ticket := range run.tickets {
			if ticket.Label != event.Label {
				continue
			}
			ticket.Running = true
			ticket.Paused = false
			ticket.PauseKind = ""
			ticket.PauseReason = ""
			run.tickets[identifier] = ticket
			break
		}
	case ralphloop.LiveEventIterationFinished:
		ticket, ok := run.tickets[event.Identifier]
		if ok {
			ticket.Running = false
			ticket.Paused = false
			ticket.Completed = true
			ticket.PauseKind = ""
			ticket.PauseReason = ""
			ticket.ContextTokens = 0
			run.tickets[event.Identifier] = ticket
			run.done++
		}
	case ralphloop.LiveEventEpicComplete:
		run.done = event.Completed
		run.state = RunStateCompleted
	}
}

func (r *loopRegistry) runSnapshot(epicName string) (RunSnapshot, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.snapshots[epicName]
	if run == nil {
		return RunSnapshot{}, false
	}
	return copyRunSnapshot(epicName, run, r.paused), true
}

func (r *loopRegistry) runSnapshots() []RunSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.snapshots))
	for name := range r.snapshots {
		names = append(names, name)
	}
	sort.Strings(names)
	snapshots := make([]RunSnapshot, len(names))
	for i, name := range names {
		snapshots[i] = copyRunSnapshot(name, r.snapshots[name], r.paused)
	}
	return snapshots
}

func copyRunSnapshot(epicName string, run *epicRun, queuePaused bool) RunSnapshot {
	tickets := make(map[string]RunTicketSnapshot, len(run.tickets))
	contextTokens := 0
	paused := queuePaused && run.state == RunStateRunning
	for identifier, ticket := range run.tickets {
		tickets[identifier] = ticket
		contextTokens += ticket.ContextTokens
		paused = paused || ticket.Paused
	}
	return RunSnapshot{
		EpicName:      epicName,
		State:         run.state,
		Done:          run.done,
		Total:         run.total,
		ContextTokens: contextTokens,
		Paused:        paused,
		FinalError:    run.finalError,
		Tickets:       tickets,
		StartedAt:     run.startedAt,
	}
}

func (r *loopRegistry) gateFor(epicName string) *ralphloop.Gate {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run := r.runs[epicName]; run != nil {
		return run.gate
	}
	return nil
}

func (r *loopRegistry) pause() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paused = true
	for _, run := range r.runs {
		run.gate.Pause(ralphloop.QueuePauseLabel, "queue paused")
	}
}

func (r *loopRegistry) resume() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paused = false
	for _, run := range r.runs {
		run.gate.ForceResume(ralphloop.QueuePauseLabel)
	}
}

func (r *loopRegistry) isPaused() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.paused
}

func (r *loopRegistry) finish(epicName string, err error) {
	r.mu.Lock()
	run := r.runs[epicName]
	r.mu.Unlock()
	if run != nil && run.sink != nil {
		run.sink.Close()
		<-run.drainDone
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if run != nil {
		run.state = RunStateCompleted
		if err != nil {
			run.state = RunStateFailed
			run.finalError = err.Error()
		}
		run.sink = nil
	}
	delete(r.runs, epicName)
	if err != nil {
		r.lastErr[epicName] = err
	} else {
		delete(r.lastErr, epicName)
	}
}

func (r *loopRegistry) lastError(epicName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErr[epicName]
}

// EpicProgress reports one running epic's name and landed/total ticket count,
// for LoopStatusAll's cross-tab overlay listing (ticket 05).
type EpicProgress struct {
	Name        string
	Done, Total int
}

// LoopStatusAll reports every currently-running epic (ticket 05's
// multi-epic-aware cross-tab status overlay, superseding the single-epic
// LoopStatus), sorted by name for deterministic rendering order. It polls the
// same way this package's own OnPageActivated/handleImplementPoll do rather
// than subscribing directly, since the app shell only routes tea.Msgs to the
// active page.
func LoopStatusAll() []EpicProgress {
	ralphLoopRegistry.mu.Lock()
	defer ralphLoopRegistry.mu.Unlock()
	names := make([]string, 0, len(ralphLoopRegistry.runs))
	for name := range ralphLoopRegistry.runs {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]EpicProgress, len(names))
	for i, name := range names {
		run := ralphLoopRegistry.runs[name]
		out[i] = EpicProgress{Name: name, Done: run.done, Total: run.total}
	}
	return out
}

// CanQuit implements the app shell's quit-guard duck type (see
// ui/app/model_quit.go): it blocks quitting gx while this process has any
// epic's ralph-loop in flight, since an interrupted loop can leave the
// worktree mid-cherry-pick. This is warn-then-allow, not a hard block —
// reconcile.go recovers an interrupted loop on the next run, so there's no
// correctness reason to prevent quitting outright.
func (m Model) CanQuit() bool {
	return !ralphLoopRegistry.isRunning()
}

// isRunning reports whether any epic currently has a run in flight.
func (r *loopRegistry) isRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs) > 0
}

func (r *loopRegistry) availableSlots() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.paused {
		return 0
	}
	return max(r.maxConcurrent-len(r.runs), 0)
}

// IsLoopRunning reports whether a ralph-loop launched from this process is
// currently in flight for any epic, regardless of which worktree it
// targets. The app shell (ticket 05) uses this to force the tickets tab
// into --all scope while a loop is running, since the loop keeps going
// against its own worktree even after the user navigates the worktree
// cursor elsewhere.
func IsLoopRunning() bool {
	return ralphLoopRegistry.isRunning()
}

// isRunningEpic reports whether epicName has a run in flight, so a poll loop
// knows to keep going without accessing the run's stream directly.
func (r *loopRegistry) isRunningEpic(epicName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.runs[epicName]
	return ok
}

// runningEpicNames reports every epic currently mid-run, sorted by name, so a
// reactivated tab can fan out over all of them instead of recovering just one
// (see OnPageActivated).
func (r *loopRegistry) runningEpicNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.runs))
	for name := range r.runs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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

// implementSyncMsg reports every epic ralphLoopRegistry has running, as
// observed when this tab (re)gained focus (see OnPageActivated), so a Model
// that missed completion messages for one or more epics while another tab was
// active can catch up on all of them at once.
type implementSyncMsg struct {
	runningEpics []string
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

// handleImplementKey applies the checked selection to the queue. With no
// ralph-loop active (ticket 11), it replaces the not-yet-started queue
// entries with the checked selection directly and switches to the Queue tab
// — no confirmation, since nothing running/done is at risk. With a loop
// active, ticket 12 gates this behind a confirmation instead.
func (m Model) handleImplementKey() (tea.Model, tea.Cmd) {
	if len(m.checked) == 0 {
		return m, notify.Info("check at least one ticket to build an execution plan")
	}
	worktreeRoot := m.worktreeRoot
	if !IsLoopRunning() {
		if err := m.replaceQueuedSelection(); err != nil {
			return m, notify.Error("save queue: " + err.Error())
		}
		return m, cmdOpenQueueTab(worktreeRoot)
	}
	count := len(m.checked)
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    fmt.Sprintf("Open the execution plan for %d checked ticket(s)?", count),
		AcceptCmd: cmdOpenQueueTab(worktreeRoot),
	})
	return m, nil
}

// replaceQueuedSelection applies ticket 11's "i" replace logic: within this
// tab's own worktree scope (mirroring scopedQueueSnapshot), every pending
// (not-yet-started) queue entry is dropped and replaced by the current
// checked selection. Running/done/errored entries — and anything outside
// scope, which this tab can't see to safely decide about — are left exactly
// as they are, whether or not they're still checked.
func (m *Model) replaceQueuedSelection() error {
	snapshot := m.queueStore.Snapshot()
	next := make(map[string]queueItemStatus, len(snapshot.Status))
	order := make(map[string]uint64, len(snapshot.Order))
	for path, status := range snapshot.Status {
		if status == queueStatusPending && m.inScope(path) {
			continue
		}
		next[path] = status
		order[path] = snapshot.Order[path]
	}
	for path := range m.checked {
		if _, exists := next[path]; exists {
			continue
		}
		next[path] = queueStatusPending
		order[path] = m.checkOrder[path]
	}
	if err := m.queueStore.Replace(next, order); err != nil {
		return err
	}
	m.refreshQueueSnapshot()
	return nil
}

func cmdOpenQueueTab(worktreeRoot string) tea.Cmd {
	return nav.Switch(nav.ViewState{Tab: nav.TabQueue, WorktreeRoot: worktreeRoot})
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
	return renderImplementAgentMenu(prompt, m.implementAgentMenu)
}

func renderImplementAgentMenu(prompt string, menu components.MenuState) string {
	return components.RenderMenuModal(
		"Implement Epic",
		prompt,
		menu,
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

func (m Model) handleConfirmMouseUpdate(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.UpdateMouse(msg, m.width, m.width, m.height)
	m.confirm = next
	return m, cmd
}

func (m Model) handleImplementStarted(msg implementStartedMsg) (tea.Model, tea.Cmd) {
	m.implementEpic = msg.epicName
	m.syncRunSnapshot(msg.epicName)
	return m, tea.Batch(m.implementSpinner.Tick, cmdPollImplement(msg.epicName))
}

// handleImplementPoll projects active registry state and reloads disk state
// after completion. Errors remain in the registry for every observer.
func (m Model) handleImplementPoll(msg implementPollMsg) (tea.Model, tea.Cmd) {
	if ralphLoopRegistry.isRunningEpic(msg.epicName) {
		m.syncRunSnapshot(msg.epicName)
		return m, cmdPollImplement(msg.epicName)
	}
	m.clearLiveTrackingFor(msg.epicName)
	return m, tea.Batch(implementFinishedNotifyCmd(msg.epicName), m.cmdLoad())
}

// implementFinishedNotifyCmd reports epicName's just-finished run: an error
// toast if ralphloop.Run returned one, otherwise the plain completion toast.
func implementFinishedNotifyCmd(epicName string) tea.Cmd {
	if err := ralphLoopRegistry.lastError(epicName); err != nil {
		return notify.Error(fmt.Sprintf("ralph-loop failed for epic %q: %v", epicName, err))
	}
	return notify.Info(fmt.Sprintf("ralph-loop finished for epic %q", epicName))
}

// handleImplementSync answers OnPageActivated's resync Cmd: it reconciles
// every epic this Model was tracking against ralphLoopRegistry's live state,
// which may have changed while this tab was in the background (a plain
// tea.Msg sent to a backgrounded page is dropped by the app shell, so the
// model that launched a run can't rely on ever seeing its own completion
// message if the user switched away in the meantime) — and it also starts
// tracking any epic that's running but that this Model instance never saw
// start, e.g. one launched before this Model was rebuilt by a tab switch.
func (m Model) handleImplementSync(msg implementSyncMsg) (tea.Model, tea.Cmd) {
	running := make(map[string]bool, len(msg.runningEpics))
	var cmds []tea.Cmd
	for _, epicName := range msg.runningEpics {
		running[epicName] = true
		m.syncRunSnapshot(epicName)
		cmds = append(cmds, cmdPollImplement(epicName))
	}
	if len(msg.runningEpics) > 0 {
		cmds = append(cmds, m.implementSpinner.Tick)
	}

	finished := make([]string, 0, len(m.implementingEpics))
	for epicName := range m.implementingEpics {
		if !running[epicName] {
			finished = append(finished, epicName)
		}
	}
	sort.Strings(finished)
	for _, epicName := range finished {
		m.clearLiveTrackingFor(epicName)
		cmds = append(cmds, implementFinishedNotifyCmd(epicName))
	}
	if len(finished) > 0 {
		cmds = append(cmds, m.cmdLoad())
	}

	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleImplementSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if len(m.implementingEpics) == 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.implementSpinner, cmd = m.implementSpinner.Update(msg)
	return m, cmd
}

// OnPageActivated implements the app shell's pageActivationAware duck-type
// (see ui/app/model_tabs.go), firing every time this tab (re)gains focus —
// including the very first time. It fires an implementSyncMsg listing every
// epic ralphLoopRegistry currently has running, so this Model can recover
// every running epic's live state deterministically — including a completion
// it missed, or an epic it never even saw start — without opening an event
// reader of its own or depending on messages that arrived while another tab
// was active.
func (m Model) OnPageActivated() tea.Cmd {
	return func() tea.Msg {
		return implementSyncMsg{runningEpics: ralphLoopRegistry.runningEpicNames()}
	}
}

// cmdStartImplement launches the producer while the registry owns the sole
// event consumer and publishes durable snapshots to presentation models.
func (m Model) cmdStartImplement(epicName string, agent ralphloop.AgentKind, done, total int) tea.Cmd {
	return cmdStartImplement(
		m.worktreeRoot, epicName, agent, done, total,
		m.settings.MaxConcurrentTicketsPerEpic(), nil, m.settings.Notifications.Telegram,
	)
}

func cmdStartImplement(
	worktreeRoot string,
	epicName string,
	agent ralphloop.AgentKind,
	done, total int,
	maxParallel int,
	ticketIDs []string,
	telegram config.TelegramConfig,
) tea.Cmd {
	return func() tea.Msg {
		sink, ok := ralphLoopRegistry.tryStart(epicName, done, total)
		if !ok {
			return implementFailedMsg{err: fmt.Errorf("a ralph-loop is already running")}
		}
		opts, err := buildImplementRunOptionsForTickets(worktreeRoot, epicName, agent, maxParallel, ticketIDs)
		if err != nil {
			ralphLoopRegistry.finish(epicName, err)
			return implementFailedMsg{err: err}
		}
		opts.Gate = ralphLoopRegistry.gateFor(epicName)
		var runSink ralphloop.EventSink = sink
		if telegram.BotToken != "" {
			runSink = ralphloop.NewTelegramEventSink(sink, telegram.BotToken, telegram.ChatID)
		}
		go func() {
			err := runRalphLoop(opts, ralphloop.DefaultDeps(), runSink)
			ralphLoopRegistry.finish(epicName, err)
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

func buildImplementRunOptions(worktreeRoot, epicName string, agent ralphloop.AgentKind) (ralphloop.RunOptions, error) {
	return buildImplementRunOptionsForTickets(
		worktreeRoot, epicName, agent,
		ui.Settings{}.MaxConcurrentTicketsPerEpic(), nil,
	)
}

func buildImplementRunOptionsForTickets(
	worktreeRoot, epicName string,
	agent ralphloop.AgentKind,
	maxParallel int,
	ticketIDs []string,
) (ralphloop.RunOptions, error) {
	repo, err := git.FindRepo(worktreeRoot)
	if err != nil {
		return ralphloop.RunOptions{}, err
	}
	return ralphloop.RunOptions{
		EpicName:    epicName,
		Agent:       agent,
		RepoDir:     repo.Root,
		ScratchDir:  filepath.Join(worktreeRoot, ".scratch"),
		MaxParallel: max(maxParallel, 1),
		TicketIDs:   ticketIDs,
	}, nil
}
