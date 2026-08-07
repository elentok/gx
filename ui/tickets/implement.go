package tickets

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
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
	// scope is written once, by RunOptions.OnScopeResolved shortly after the
	// run starts (see cmdStartImplement) — until then it's the zero RunScope,
	// on which Add is a documented no-op.
	scope       ralphloop.RunScope
	state       RunState
	finalError  string
	tickets     map[string]RunTicketSnapshot
	startedAt   time.Time
	// pendingNotifyCloses queues reattach-scan notification ids to close,
	// populated by reduceLiveEvent (running in the event-drain goroutine,
	// which has no way to send a tea.Cmd itself) and drained into an actual
	// notify.Close cmd by the next syncRunSnapshot poll on the active tab.
	pendingNotifyCloses []string
	// holdsAttach marks a run as one of the ones counted toward
	// loopRegistry.attachCount, so finish only releases the attach lock once
	// the run that actually incremented it ends.
	holdsAttach bool
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
	StartedAt     time.Time
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
	// attachCount/attachScratchDir reference-count this process's hold on the
	// per-repo attach lock (ticket 05): the lock is acquired once, on the
	// first tryStart to pass a scratchDir while attachCount is 0, and
	// released once attachCount drops back to 0 in finish.
	attachCount      int
	attachScratchDir string
	// attachErr carries the reason the most recent tryStart was rejected by
	// the attach lock specifically (as opposed to same-process double-start
	// or capacity), for the caller to surface instead of the generic
	// "already running" message. Cleared at the start of every tryStart.
	attachErr error
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
//
// scratchDir is variadic-as-optional so the many pre-existing callers
// (mostly tests exercising slot/pause/concurrency logic, not the attach
// lock) don't need updating: passed, it's the repo's `.scratch` dir and
// tryStart acquires ticket 05's per-repo attach lock the first time
// attachCount is 0; omitted, the run skips attach-lock participation
// entirely.
func (r *loopRegistry) tryStart(epicName string, done, total int, scratchDir ...string) (*ralphloop.ChannelEventSink, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attachErr = nil
	if r.paused {
		return nil, false
	}
	if _, exists := r.runs[epicName]; exists {
		return nil, false
	}
	if len(r.runs) >= r.maxConcurrent {
		return nil, false
	}

	dir := ""
	if len(scratchDir) > 0 {
		dir = scratchDir[0]
	}
	holdsAttach := false
	if dir != "" {
		if r.attachCount == 0 {
			foreignPID, ok, err := acquireAttachLock(dir)
			if err != nil {
				r.attachErr = err
				return nil, false
			}
			if !ok {
				r.attachErr = fmt.Errorf("a ralph-loop is already running (attached by process %d)", foreignPID)
				return nil, false
			}
			r.attachScratchDir = dir
		}
		r.attachCount++
		holdsAttach = true
	}

	sink := ralphloop.NewChannelEventSink()
	run := &epicRun{
		drainDone: make(chan struct{}),
		done:      done, total: total, sink: sink, gate: ralphloop.NewGate(),
		state: RunStateRunning, tickets: map[string]RunTicketSnapshot{},
		startedAt: time.Now(), holdsAttach: holdsAttach,
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
			StartedAt:  time.Now(),
		}
		if event.Kind == ralphloop.LiveEventTicketReattached {
			run.pendingNotifyCloses = append(run.pendingNotifyCloses, reattachNotifyID(epicName, event.Identifier))
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
			run.pendingNotifyCloses = append(run.pendingNotifyCloses, reattachNotifyID(epicName, identifier))
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

// drainPendingNotifyCloses hands back and clears epicName's queued
// reattach-scan notification ids (see epicRun.pendingNotifyCloses) so the
// caller can turn them into notify.Close cmds exactly once.
func (r *loopRegistry) drainPendingNotifyCloses(epicName string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.snapshots[epicName]
	if run == nil || len(run.pendingNotifyCloses) == 0 {
		return nil
	}
	ids := run.pendingNotifyCloses
	run.pendingNotifyCloses = nil
	return ids
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

// setScope records epicName's resolved RunScope, called back from
// RunOptions.OnScopeResolved once Run has loaded the epic and resolved it.
func (r *loopRegistry) setScope(epicName string, scope ralphloop.RunScope) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run := r.runs[epicName]; run != nil {
		run.scope = scope
	}
}

// scopeFor returns epicName's live RunScope, for "add to queue" (ticket 10)
// to widen via RunScope.Add. ok is false once the epic is no longer running.
func (r *loopRegistry) scopeFor(epicName string) (ralphloop.RunScope, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[epicName]
	if run == nil {
		return ralphloop.RunScope{}, false
	}
	return run.scope, true
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

// holdsAttach reports whether this process currently holds the per-repo
// attach lock (ticket 05), for SelfAttached's tab-label signal.
func (r *loopRegistry) holdsAttach() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attachCount > 0
}

// SelfAttached reports whether this process holds the per-repo attach lock —
// the Queue tab label's "(attached)" suffix (ticket 07) is self-only and
// never reflects a foreign process's attachment (surfaced separately via
// ForeignAttachPID and ticket 05's hard-block error).
func SelfAttached() bool {
	return ralphLoopRegistry.holdsAttach()
}

// ForeignAttachPID reports the pid of a live foreign process currently
// holding scratchDir's attach lock, or 0 when unattached, the lock is stale,
// or this process itself holds it. It shells out to `ps` (via
// attachLockIsStale) so callers should poll it on a refresh cadence rather
// than call it on every render.
func ForeignAttachPID(scratchDir string) int {
	if ralphLoopRegistry.holdsAttach() {
		return 0
	}
	info, err := readAttachLock(attachLockPath(scratchDir))
	if err != nil {
		return 0
	}
	if attachLockIsStale(info) {
		return 0
	}
	return info.PID
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
		if run.holdsAttach {
			r.attachCount--
			if r.attachCount <= 0 {
				releaseAttachLock(r.attachScratchDir)
				r.attachCount = 0
				r.attachScratchDir = ""
			}
		}
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

// takeAttachError returns and clears the reason the most recent tryStart was
// rejected by the attach lock, or nil if that rejection had another cause.
func (r *loopRegistry) takeAttachError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.attachErr
	r.attachErr = nil
	return err
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

// handleReplaceQueueKey applies ticket 10's "r" ("Replace queue") action: with
// no ralph-loop live anywhere in this process, it replaces the not-yet-started
// queue entries with the checked selection directly and switches to the
// Queue tab — no confirmation, since nothing running/done is at risk. Once
// any epic has a live run, replacing the queue risks stomping in-flight
// work, so "r" is entirely disabled instead — process-wide, regardless of
// which epic the checked tickets belong to — and the pending selection is
// left untouched.
func (m Model) handleReplaceQueueKey() (tea.Model, tea.Cmd) {
	if IsLoopRunning() {
		return m, notify.Info("Can't replace a live queue")
	}
	if len(m.checked) == 0 {
		return m, notify.Info("check at least one ticket to build an execution plan")
	}
	worktreeRoot := m.worktreeRoot
	if err := m.replaceQueuedSelection(); err != nil {
		return m, notify.Error("save queue: " + err.Error())
	}
	return m, cmdOpenQueueTab(worktreeRoot)
}

// handleAddToQueueKey applies ticket 10's "a" ("Add to queue") action: the
// epic under the cursor must already have a live run, and the checked
// tickets belonging to it are added to that run's scope after confirmation —
// widening a frozen scope via ralphloop.RunScope.Add (ticket 09) so each
// becomes claimable on the run's next iteration. Unlike "r", "a" always
// confirms first, naming the count about to be added.
func (m Model) handleAddToQueueKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok {
		return m, nil
	}
	epic := m.epics[r.epicIdx]
	if !ralphLoopRegistry.isRunningEpic(epic.Name) {
		return m, notify.Info(fmt.Sprintf("epic %q isn't running", epic.Name))
	}
	ticketIDs := checkedTicketIDsForEpic(epic, m.checked)
	if len(ticketIDs) == 0 {
		return m, notify.Info("check at least one ticket in the epic to add")
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    fmt.Sprintf("Add %d ticket(s) to the live queue?", len(ticketIDs)),
		AcceptCmd: cmdAddToLiveQueue(epic.Name, ticketIDs),
	})
	return m, nil
}

// checkedTicketIDsForEpic collects DisplayNumber identifiers (the form
// ralphloop.RunScope.Add expects) for epic's checked, not-yet-done tickets —
// a done ticket has nothing left to add to a live run's scope.
func checkedTicketIDsForEpic(epic tickets.Epic, checked map[string]bool) []string {
	var ids []string
	for _, t := range epic.Tickets {
		if !checked[t.Path] || epic.RenderedStatus(t) == tickets.StatusDone {
			continue
		}
		ids = append(ids, t.DisplayNumber())
	}
	return ids
}

// cmdAddToLiveQueue widens epicName's live RunScope to include ticketIDs
// once the "a" confirmation modal is accepted.
func cmdAddToLiveQueue(epicName string, ticketIDs []string) tea.Cmd {
	return func() tea.Msg {
		scope, ok := ralphLoopRegistry.scopeFor(epicName)
		if !ok {
			return notify.Error(fmt.Sprintf("epic %q is no longer running", epicName))()
		}
		scope.Add(ticketIDs...)
		return notify.Info(fmt.Sprintf("added %d ticket(s) to epic %q", len(ticketIDs), epicName))()
	}
}

// replaceQueuedSelection applies ticket 10's "r" replace logic: every pending
// (not-yet-started) queue entry is dropped and replaced by the current
// checked selection. Running/done/errored entries are left exactly as they
// are, whether or not they're still checked. Ticket 15's
// EnqueueAndClearChecked also clears every just-queued path from the
// Tickets tab's independent checked set in the same atomic write, so the
// checkboxes visually reset the moment their tickets are queued.
func (m *Model) replaceQueuedSelection() error {
	snapshot := m.queueStore.Snapshot()
	next := make(map[string]queueItemStatus, len(snapshot.Status))
	order := make(map[string]uint64, len(snapshot.Order))
	for path, status := range snapshot.Status {
		if status == queueStatusPending {
			continue
		}
		next[path] = status
		order[path] = snapshot.Order[path]
	}
	clearedPaths := make([]string, 0, len(m.checked))
	for path := range m.checked {
		clearedPaths = append(clearedPaths, path)
		if m.isTicketDone(path) {
			continue
		}
		if _, exists := next[path]; exists {
			continue
		}
		next[path] = queueStatusPending
		order[path] = m.checkOrder[path]
	}
	if err := m.queueStore.EnqueueAndClearChecked(next, order, clearedPaths); err != nil {
		return err
	}
	m.refreshQueueSnapshot()
	return nil
}

// isTicketDone reports whether path's ticket is already tickets.StatusDone
// within m.epics — a done ticket has nothing left to implement, so
// replaceQueuedSelection excludes it from the checked selection it enqueues.
func (m *Model) isTicketDone(path string) bool {
	for _, epic := range m.epics {
		for _, t := range epic.Tickets {
			if t.Path == path {
				return epic.RenderedStatus(t) == tickets.StatusDone
			}
		}
	}
	return false
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
	closeCmd := m.syncRunSnapshot(msg.epicName)
	return m, tea.Batch(m.implementSpinner.Tick, cmdPollImplement(msg.epicName), closeCmd)
}

// handleImplementPoll projects active registry state and reloads disk state
// after completion. Errors remain in the registry for every observer.
func (m Model) handleImplementPoll(msg implementPollMsg) (tea.Model, tea.Cmd) {
	if ralphLoopRegistry.isRunningEpic(msg.epicName) {
		closeCmd := m.syncRunSnapshot(msg.epicName)
		return m, tea.Batch(cmdPollImplement(msg.epicName), closeCmd)
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
		cmds = append(cmds, m.syncRunSnapshot(epicName), cmdPollImplement(epicName))
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
	syncCmd := func() tea.Msg {
		return implementSyncMsg{runningEpics: ralphLoopRegistry.runningEpicNames()}
	}
	return tea.Batch(syncCmd, m.cmdReattachScan())
}

// cmdStartImplement launches the producer while the registry owns the sole
// event consumer and publishes durable snapshots to presentation models.
func (m Model) cmdStartImplement(epicName string, agent ralphloop.AgentKind, done, total int) tea.Cmd {
	return cmdStartImplement(
		m.worktreeRoot, epicName, agent, done, total,
		m.settings.MaxConcurrentTicketsPerEpic(), nil, m.settings.Notifications, m.settings.ImplementSkill(),
	)
}

func cmdStartImplement(
	worktreeRoot string,
	epicName string,
	agent ralphloop.AgentKind,
	done, total int,
	maxParallel int,
	ticketIDs []string,
	notifications config.NotificationsConfig,
	skill string,
) tea.Cmd {
	return func() tea.Msg {
		sink, ok := ralphLoopRegistry.tryStart(epicName, done, total, scratchDirFor(worktreeRoot))
		if !ok {
			if attachErr := ralphLoopRegistry.takeAttachError(); attachErr != nil {
				return implementFailedMsg{err: attachErr}
			}
			return implementFailedMsg{err: fmt.Errorf("a ralph-loop is already running")}
		}
		opts, err := buildImplementRunOptionsForTickets(worktreeRoot, epicName, agent, maxParallel, ticketIDs, skill)
		if err != nil {
			ralphLoopRegistry.finish(epicName, err)
			return implementFailedMsg{err: err}
		}
		opts.Gate = ralphLoopRegistry.gateFor(epicName)
		opts.OnScopeResolved = func(scope ralphloop.RunScope) {
			ralphLoopRegistry.setScope(epicName, scope)
		}
		var runSink ralphloop.EventSink = sink
		if notifications.Telegram.BotToken != "" {
			runSink = ralphloop.NewTelegramEventSink(runSink, notifications.Telegram.BotToken, notifications.Telegram.ChatID)
		}
		if notifications.Slack.WebhookURL != "" {
			runSink = ralphloop.NewSlackEventSink(runSink, notifications.Slack.WebhookURL)
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
	settings := ui.Settings{}
	return buildImplementRunOptionsForTickets(
		worktreeRoot, epicName, agent,
		settings.MaxConcurrentTicketsPerEpic(), nil, settings.ImplementSkill(),
	)
}

func buildImplementRunOptionsForTickets(
	worktreeRoot, epicName string,
	agent ralphloop.AgentKind,
	maxParallel int,
	ticketIDs []string,
	skill string,
) (ralphloop.RunOptions, error) {
	repo, err := git.FindRepo(worktreeRoot)
	if err != nil {
		return ralphloop.RunOptions{}, err
	}
	return ralphloop.RunOptions{
		EpicName:    epicName,
		Agent:       agent,
		Skill:       skill,
		RepoDir:     repo.Root,
		ScratchDir:  repo.ScratchRoot(),
		MaxParallel: max(maxParallel, 1),
		TicketIDs:   ticketIDs,
	}, nil
}
