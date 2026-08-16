package tickets

import (
	"fmt"
	"sort"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tree"
)

// QueueModel renders a checked selection as dependency-aware epic waves.
type QueueModel struct {
	executionStartedAt   time.Time
	executionCompletedAt time.Time
	// executionTickets is this run's captured ticket scope (epicName/identifier
	// keys), fixed at kickoff so progress totals don't shift if the checked
	// selection is edited while the run is active (ticket 20).
	executionTickets map[string]bool
	// runTicketIDs is each running epic's captured ticket-ID subset, fixed at
	// kickoff alongside executionTickets — the lifecycle-status transitions
	// (running/done/errored) below are driven from this rather than
	// re-deriving from the live checked set.
	runTicketIDs map[string][]string
	now          func() time.Time
	worktreeRoot string
	settings     ui.Settings
	checked      map[string]bool
	checkOrder   map[string]uint64
	// queueStatus mirrors m.checked's keys with each ticket's durable
	// lifecycle status (queue_state.go), read from queueStore so rendering
	// doesn't duplicate completion into its own bookkeeping (ticket 20).
	queueStatus map[string]queueItemStatus
	queueStore  *QueueStore
	// live mirrors Model.live (model_live.go): epicName -> ticket identifier
	// -> in-memory orchestrator state, synced from the registry alongside
	// queueStatus so the Queue tab's rows can render the same running/paused
	// spinner+phase presentation as the Tickets tab (renderLiveTicketRow).
	live             map[string]map[string]liveTicketState
	implementSpinner spinner.Model

	width, height int
	ready         bool
	loaded        bool
	epics         []tickets.Epic
	candidates    map[string]bool
	// autoRefreshStarted guards cmdAutoRefresh's self-perpetuating poll loop
	// (auto_refresh.go) against being started more than once per QueueModel
	// instance, mirroring Model.autoRefreshStarted.
	autoRefreshStarted bool

	// queueTree owns the Queue tab's selection/scroll/collapse state
	// (tree.Model[queueNode], see queue_rows.go's buildQueueEntries).
	queueTree tree.Model[queueNode]

	// entriesCache memoizes buildQueueEntries' output across renders. It's a
	// pointer so the cache survives QueueModel being copied by value on every
	// Update: buildQueueEntries only depends on m.epics/m.checked/
	// m.hideComplete/collapsed-IDs (see queueEntriesCache), everything else
	// live (running-epic elapsed time, spinner frame, parked/stalled state)
	// is read straight from the model by queueRenderOpts' Label callback at
	// draw time, not baked into the cached entries — so reusing a cached
	// tree when none of those four inputs changed is safe even mid-run.
	// Cuts the CPU cost of cmdAutoRefresh's 2s poll (auto_refresh.go)
	// rebuilding the full tree from scratch every render even when nothing
	// on disk changed.
	entriesCache *queueEntriesCache

	implementAgentMenuOpen bool
	implementAgentMenu     components.MenuState
	// actionsMenu backs the "m"-triggered suggested-actions menu (see
	// queue_actions_menu.go), mirroring the Tickets tab's own
	// Model.actionsMenu — a deliberate, narrow exception to this tab's
	// otherwise read-only selection (ticket 08).
	actionsMenu  actionsMenuModel
	pendingEpics []checkedEpicPlan
	runningEpics map[string]bool
	runningAgent ralphloop.AgentKind
	paused       bool
	// foreignAttachPID is the pid of a different process currently holding
	// the per-repo attach lock (ticket 05), refreshed alongside the epics
	// reload (cmdLoadQueue) since checking it shells out to `ps`. Zero when
	// unattached or when this process itself holds the lock — see
	// ForeignAttachPID.
	foreignAttachPID int

	// search backs "/"-triggered filtering over buildQueueEntries, mirroring
	// the Tickets tab's own m.search (see ui/tickets/search.go).
	search search.Model

	// confirm backs the "C"/"c" clear keymaps and the "x" cascade-delete keymap
	// (handleQueueKey) — the Queue tab is read-only for selection (ticket 08),
	// so this is the only modal this tab opens outside the agent-picker menu.
	confirm confirm.Model

	// hideComplete backs the "tc" chord (ticket 09): when true,
	// buildQueueEntries omits StatusDone tickets from the rendered row list,
	// independent of the "c"/"C" clear keymaps (which mutate the queue store,
	// not visibility) and independent of epicWaves' plan validation, which
	// must keep considering hidden-but-still-queued tickets.
	hideComplete bool
	// keys dispatches the "tc" chord above through ui/keys.Manager so a key
	// typed right after an unconsumed "t" falls through to its own normal
	// action instead of being swallowed (ticket 16).
	keys keys.Manager
	help help.Model

	// previewFocus backs the preview panel's scroll/search machinery, shared
	// with the Tickets tab (see preview_focus.go) — ticket 11 gave the Queue
	// tab real scroll/search instead of the old truncate-only preview, and
	// ticket 12 wires its promoted focus field up to "l"/"right"/"enter" and
	// "h"/"left"/"esc" (see the ExpandNoop/OpenSelected handling in
	// handleQueueKey and handleQueuePreviewKey in queue_preview.go),
	// mirroring the Tickets tab's own focus-toggle.
	previewFocus
}

func NewQueueModel(worktreeRoot string, settings ui.Settings, checked map[string]bool, extraKeys keys.Manager, orders ...map[string]uint64) QueueModel {
	if checked == nil {
		checked = map[string]bool{}
	}
	checkOrder := map[string]uint64{}
	if len(orders) > 0 && orders[0] != nil {
		checkOrder = orders[0]
	}
	sp := spinner.New()
	sp.Spinner = TicketProgressSpinner
	km := newQueueKeysManager()
	queueTree := tree.NewModel[queueNode]()
	queueTree.SetIsSelectable(func(n queueNode) bool {
		switch n.kind {
		case nodeEpicSeparator, nodeEpicStatus, nodeEpicContext, nodeEpicError:
			return false
		default:
			return true
		}
	})
	return QueueModel{
		executionTickets:   map[string]bool{},
		runTicketIDs:       map[string][]string{},
		now:                time.Now,
		worktreeRoot:       worktreeRoot,
		settings:           settings,
		checked:            checked,
		checkOrder:         checkOrder,
		queueStatus:        map[string]queueItemStatus{},
		live:               map[string]map[string]liveTicketState{},
		implementSpinner:   sp,
		implementAgentMenu: newRunStartAgentMenu(),
		runningEpics:       map[string]bool{},
		paused:             ralphLoopRegistry.isPaused(),
		confirm:            confirm.New(),
		search:             search.NewModel(),
		keys:               km,
		queueTree:          queueTree,
		entriesCache:       &queueEntriesCache{},
		help:               help.NewModel(help.BuildSections(km, *queueTree.Keys(), extraKeys)),
		previewFocus:       newPreviewFocus(),
	}
}

func NewQueueModelWithStore(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager, store *QueueStore) QueueModel {
	snapshot := store.Snapshot()
	m := NewQueueModel(worktreeRoot, settings, snapshot.Checked, extraKeys, snapshot.Order)
	m.queueStore = store
	m.queueStatus = snapshot.Status
	return m
}

func (m QueueModel) Init() tea.Cmd {
	return m.cmdLoadQueue()
}

type queueEpicsLoadedMsg struct {
	epics            []tickets.Epic
	err              error
	foreignAttachPID int
}

func (m QueueModel) cmdLoadQueue() tea.Cmd {
	scratchDir := scratchDirFor(m.worktreeRoot)
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		return queueEpicsLoadedMsg{epics: epics, err: err, foreignAttachPID: ForeignAttachPID(scratchDir)}
	}
}

// Update delegates to updateInner then re-syncs the preview viewport,
// mirroring the Tickets tab's own Update/syncPreviewViewport split (see
// model.go) so every message that can move the selection, resize the
// panels, or reload data doesn't need to remember to do it itself.
func (m QueueModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.updateInner(msg)
	nm := next.(QueueModel)
	nm.syncQueuePreviewViewport()
	return nm, cmd
}

func (m QueueModel) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.queueStore != nil {
		snapshot := m.queueStore.Snapshot()
		m.checked = snapshot.Checked
		m.checkOrder = snapshot.Order
		m.queueStatus = snapshot.Status
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.help, _ = m.help.Update(msg)
		m.queueTree.SetVisibleHeight(m.queueViewportHeight() - queueHeaderReservedLines)
		return m, nil
	case queueEpicsLoadedMsg:
		if err := autoQueueForkedChildren(m.epics, msg.epics, m.queueStore); err != nil {
			return m, notify.Error("save queue: " + err.Error())
		}
		if err := autoQueueNewEpicSiblings(m.epics, msg.epics, m.queueStore); err != nil {
			return m, notify.Error("save queue: " + err.Error())
		}
		if m.queueStore != nil {
			snapshot := m.queueStore.Snapshot()
			m.checked = snapshot.Checked
			m.checkOrder = snapshot.Order
			m.queueStatus = snapshot.Status
		}
		firstLoad := !m.loaded
		m.loaded = true
		m.epics = msg.epics
		m.foreignAttachPID = msg.foreignAttachPID
		m.candidates = make(map[string]bool, len(m.checked))
		for path := range m.checked {
			m.candidates[path] = true
		}
		if m.search.HasQuery() {
			m.recomputeQueueSearchMatches()
		}
		m.clampSelected()
		var cmds []tea.Cmd
		if !m.autoRefreshStarted {
			m.autoRefreshStarted = true
			cmds = append(cmds, cmdAutoRefresh())
		}
		// The initial OnPageActivated (fired from the very same tab-switch
		// batch that triggers this load, see applySwitch) can race ahead of
		// this msg — it reads the registry synchronously while epics are
		// still loading async, so cmdCheckStrandedPending would otherwise
		// scan an empty m.epics. Re-run it here, now that epics are
		// definitely loaded, but only on the first load — a checked epic
		// legitimately sits "checked, not running" between being checked and
		// the user pressing Enter, so this can't fire on every reload without
		// nagging mid-selection.
		if firstLoad {
			cmds = append(cmds, m.cmdCheckStrandedPending())
		}
		return m, tea.Batch(cmds...)

	case autoRefreshMsg:
		return m, tea.Batch(m.cmdLoadQueue(), cmdAutoRefresh())
	case implementStartedMsg:
		if m.executionStartedAt.IsZero() {
			m.executionStartedAt = m.now()
		}
		m.runningEpics[msg.epicName] = true
		m.markEpicTicketsRunning(msg.epicName)
		closeCmd := m.syncRunSnapshot(msg.epicName)
		return m, tea.Batch(cmdPollImplement(msg.epicName), m.implementSpinner.Tick, closeCmd)
	case implementPollMsg:
		snapshot, ok := ralphLoopRegistry.runSnapshot(msg.epicName)
		if ok {
			m.syncExecutionScope(msg.epicName, snapshot)
		}
		closeCmd := m.syncRunSnapshot(msg.epicName)
		if ok && snapshot.State == RunStateParked {
			delete(m.runningEpics, msg.epicName)
			return m, tea.Batch(cmdPollImplement(msg.epicName), closeCmd)
		}
		if ralphLoopRegistry.isRunningEpic(msg.epicName) {
			m.runningEpics[msg.epicName] = true
			return m, tea.Batch(cmdPollImplement(msg.epicName), closeCmd)
		}
		m.finalizeEpicTicketStatus(msg.epicName)
		delete(m.runningEpics, msg.epicName)
		delete(m.live, msg.epicName)
		executionComplete := !m.executionStartedAt.IsZero() && len(m.runningEpics) == 0 && len(m.pendingEpics) == 0 && len(ralphLoopRegistry.parkedEpics()) == 0
		startCmd := m.startAvailableEpics()
		if executionComplete {
			m.executionCompletedAt = m.now()
			return m, tea.Batch(implementFinishedNotifyCmd(msg.epicName), startCmd, m.cmdLoadQueue())
		}
		return m, tea.Batch(implementFinishedNotifyCmd(msg.epicName), startCmd)
	case implementFailedMsg:
		return m, notify.Error(msg.err.Error())
	case queueSyncMsg:
		return m.handleQueueSync(msg)
	case spinner.TickMsg:
		return m.handleQueueSpinnerTick(msg)
	case tea.MouseWheelMsg:
		if next, cmd, handled := m.help.Forward(msg); handled {
			m.help = next
			return m, cmd
		}
		return m.handleQueueMouseWheel(msg)
	case queueClearConfirmedMsg:
		return m.handleQueueClearConfirmed(msg)
	case queuePauseConfirmedMsg:
		return m.handleQueuePauseConfirmed(msg)
	case queueResumeConfirmedMsg:
		return m.handleQueueResumeConfirmed(msg)
	case budgetOverrideConfirmedMsg:
		return m.handleBudgetOverrideConfirmed(msg)
	case runStartConfirmedMsg:
		return m.handleRunStartConfirmed(msg)
	case editFileFinishedMsg:
		return m.handleEditFileFinished(msg)
	case tea.KeyPressMsg:
		if m.help.IsOpen {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}
		if m.confirm.IsOpen {
			return m.handleQueueConfirmUpdate(msg)
		}
		if m.actionsMenu.IsOpen {
			return m.handleQueueActionsMenuKey(msg)
		}
		if m.focus == focusPreview {
			return m.handleQueuePreviewKey(msg)
		}
		if nextSearch, cmd, result := m.search.Update(msg); result.Handled {
			m.search = nextSearch
			if result.QueryChanged {
				m.recomputeQueueSearchMatches()
			}
			if result.QueryChanged || result.CursorChanged {
				m.jumpToCurrentQueueMatch()
			}
			return m, cmd
		}
		return m.handleQueueKey(msg)
	case tea.MouseClickMsg:
		if m.actionsMenu.IsOpen {
			return m, nil
		}
		if m.confirm.IsOpen {
			return m.handleQueueConfirmMouseUpdate(msg)
		}
		return m.handleQueueMouseClick(msg)
	case cascadeDeleteConfirmedMsg:
		return m.handleCascadeDeleteConfirmed(msg)
	case queueDetachedLiveMsg:
		return m.handleDetachedLiveDetected(msg)
	case detachedLiveConfirmedMsg:
		return m.handleDetachedLiveConfirmed(msg)
	case queueStrandedPendingMsg:
		return m.handleStrandedPendingDetected(msg)
	case strandedPendingConfirmedMsg:
		return m.handleStrandedPendingConfirmed(msg)
	case queueActionAppliedMsg:
		return m, m.cmdLoadQueue()
	}
	return m, nil
}

func (m QueueModel) handleQueueSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if len(m.runningEpics) == 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.implementSpinner, cmd = m.implementSpinner.Update(msg)
	return m, cmd
}

// handleQueueMouseWheel scrolls whichever of the tree/preview panes the
// cursor is over, routed by ui.HoverHitTest rather than m.focus — so
// scrolling never needs a prior click/keyboard focus change, mirroring
// ui/commit's handleMouseWheel. Selection is left untouched either way.
func (m QueueModel) handleQueueMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	dir, ok := ui.WheelDirection(msg)
	if !ok {
		return m, nil
	}
	mouse := msg.Mouse()
	idx, ok := ui.HoverHitTest(mouse.X, mouse.Y, m.queueTreeRect(), m.queuePreviewRect())
	if !ok {
		return m, nil
	}
	if idx == 1 {
		var cmd tea.Cmd
		m.previewVP, cmd = m.previewVP.Update(msg)
		return m, cmd
	}
	m.queueTree.ScrollViewport(dir * ui.WheelScrollLines)
	return m, nil
}

// queueTreeRect returns the queue tree panel's absolute on-screen bounds,
// mirroring previewRect's layout math (same splitPanelWidth/splitPanelHeight
// call) for the Queue tab's own panel pair.
func (m QueueModel) queueTreeRect() ui.Rect {
	sidebarW, _ := splitPanelWidth(m.width)
	sidebarH, _ := splitPanelHeight(m.width, m.contentHeight())
	return ui.Rect{X: 0, Y: 0, W: sidebarW, H: sidebarH}
}

// queuePreviewRect wraps the shared previewRect in a ui.Rect for
// HoverHitTest.
func (m QueueModel) queuePreviewRect() ui.Rect {
	x, y, w, h := previewRect(m.width, m.contentHeight())
	return ui.Rect{X: x, Y: y, W: w, H: h}
}

// contentHeight returns the content height below the tab bar, mirroring
// Model.contentHeight for the Queue tab's own layout.
func (m QueueModel) contentHeight() int {
	return max(m.height-1, 1)
}

// previewRect returns the preview panel's absolute on-screen bounds,
// mirroring Model.previewRect for the Queue tab's own layout.
func (m QueueModel) previewRect() (x, y, w, h int) {
	return previewRect(m.width, m.contentHeight())
}

// handleQueueMouseClick selects the row under the click without triggering
// any secondary action (no confirm, no checkbox toggle). A click inside the
// preview panel's bounds instead hands focus to it (mirroring Model's
// handleSidebarMouseClick), without changing the queue selection.
func (m QueueModel) handleQueueMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	if m.previewFocus.clickToFocus(mouse, m.width, m.contentHeight()) {
		return m, nil
	}
	bodyLine := mouse.Y - 1 - queueHeaderReservedLines
	m.queueTree.SelectAtBodyLine(bodyLine)
	return m, nil
}

// queueSyncMsg reports ralphLoopRegistry's full state as observed when the
// Queue tab (re)gains focus. A poll chain's tea.Msg is dropped by the app
// shell while this tab is backgrounded (see implementPollInterval's doc
// comment), which would otherwise strand m.runningEpics/m.pendingEpics on
// whatever they were the moment focus left — including never noticing a slot
// freed up for backfill. OnPageActivated re-derives everything this tab needs
// from the registry's durable snapshots instead of trusting the messages that
// arrived while it was away.
type queueSyncMsg struct {
	snapshots []RunSnapshot
	paused    bool
}

// OnPageActivated implements the app shell's pageActivationAware duck-type
// (see ui/app/model_tabs.go): every time the Queue tab (re)gains focus,
// including the very first time, it re-syncs from the registry.
func (m QueueModel) OnPageActivated() tea.Cmd {
	return func() tea.Msg {
		return queueSyncMsg{snapshots: ralphLoopRegistry.runSnapshots(), paused: ralphLoopRegistry.isPaused()}
	}
}

// byNameSnapshot indexes the loop registry's current run snapshots by epic
// name, for callers (handleQueueSync, cmdCheckStrandedPending) that need to
// know which epics the registry already considers running before deciding
// what else to requeue.
func byNameSnapshot() map[string]RunSnapshot {
	snapshots := ralphLoopRegistry.runSnapshots()
	byName := make(map[string]RunSnapshot, len(snapshots))
	for _, s := range snapshots {
		byName[s.EpicName] = s
	}
	return byName
}

func (m QueueModel) handleQueueSync(msg queueSyncMsg) (tea.Model, tea.Cmd) {
	m.paused = msg.paused
	byName := byNameSnapshot()
	var cmds []tea.Cmd
	for _, plan := range m.checkedEpicPlans() {
		name := plan.epic.Name
		snapshot, ok := byName[name]
		if !ok {
			continue
		}
		// Baseline: the checked selection this Model instance knows about,
		// recorded even if it's reattaching to a run the registry never
		// reported an OnScopeResolved callback for yet (e.g. right after
		// tryStart). Idempotent — recordExecutionTicket dedups.
		for _, ticketID := range plan.ticketIDs {
			m.recordExecutionTicket(name, ticketID)
		}
		// Growth: widen to the run's live RunScope, so a ticket added mid-run
		// via "a" (cmdAddToLiveQueue, ralphloop.RunScope.Add) is picked up
		// without waiting for this Model's own checked selection to change.
		m.syncExecutionScope(name, snapshot)
		cmds = append(cmds, m.syncRunSnapshot(name))
		if !snapshot.StartedAt.IsZero() && (m.executionStartedAt.IsZero() || snapshot.StartedAt.Before(m.executionStartedAt)) {
			m.executionStartedAt = snapshot.StartedAt
		}
		wasRunning := m.runningEpics[name]
		if snapshot.State == RunStateParked {
			delete(m.runningEpics, name)
			cmds = append(cmds, cmdPollImplement(name))
			continue
		}
		if snapshot.State == RunStateRunning {
			m.runningEpics[name] = true
			m.markEpicTicketsRunning(name)
			cmds = append(cmds, cmdPollImplement(name))
			continue
		}
		delete(m.runningEpics, name)
		delete(m.live, name)
		if wasRunning {
			m.finalizeEpicTicketStatus(name)
			cmds = append(cmds, implementFinishedNotifyCmd(name))
		}
	}
	if len(m.runningEpics) > 0 {
		cmds = append(cmds, m.implementSpinner.Tick)
	}
	cmds = append(cmds, m.startAvailableEpics())
	if !m.executionStartedAt.IsZero() && m.executionCompletedAt.IsZero() && len(m.runningEpics) == 0 && len(m.pendingEpics) == 0 && len(ralphLoopRegistry.parkedEpics()) == 0 {
		m.executionCompletedAt = m.now()
		cmds = append(cmds, m.cmdLoadQueue())
	}
	cmds = append(cmds, cmdCheckDetachedLive(m.worktreeRoot))
	return m, tea.Batch(cmds...)
}

// syncRunSnapshot mirrors Model.syncRunSnapshot (ui/tickets/model_live.go)
// for the Queue tab's own poll loop; see closeNotifyCmd there.
func (m *QueueModel) syncRunSnapshot(epicName string) tea.Cmd {
	snapshot, ok := ralphLoopRegistry.runSnapshot(epicName)
	if !ok {
		return nil
	}
	if m.live == nil {
		m.live = map[string]map[string]liveTicketState{}
	}
	live := projectLiveTickets(snapshot)
	for identifier, ticket := range snapshot.Tickets {
		if ticket.Completed {
			if path, ok := m.ticketPathFor(epicName, identifier); ok {
				m.setItemStatus(path, queueStatusDone)
			}
		}
	}
	m.live[epicName] = live
	var reloadCmd tea.Cmd
	if ralphLoopRegistry.drainPendingReload(epicName) {
		reloadCmd = m.cmdLoadQueue()
	}
	return tea.Batch(
		closeNotifyCmd(ralphLoopRegistry.drainPendingNotifyCloses(epicName)),
		toastNotifyCmd(ralphLoopRegistry.drainPendingToasts(epicName)),
		reloadCmd,
	)
}

// ticketPathFor resolves an epicName/identifier pair (the registry's
// addressing scheme) to the ticket's file path (the queue store's key).
func (m QueueModel) ticketPathFor(epicName, identifier string) (string, bool) {
	for _, epic := range m.epics {
		if epic.Name != epicName {
			continue
		}
		for _, t := range epic.Tickets {
			if t.Identifier == identifier {
				return t.Path, true
			}
		}
	}
	return "", false
}

// setItemStatus records path's durable lifecycle status, through queueStore
// when one is wired (persisting it) or into m.queueStatus directly otherwise
// (mirroring Model.setQueueItemStatus's store/local fallback).
func (m *QueueModel) setItemStatus(path string, status queueItemStatus) {
	if path == "" {
		return
	}
	if m.queueStore != nil {
		if err := m.queueStore.SetStatus(path, status); err != nil {
			return
		}
		m.queueStatus = m.queueStore.Snapshot().Status
		return
	}
	if m.queueStatus == nil {
		m.queueStatus = map[string]queueItemStatus{}
	}
	m.queueStatus[path] = status
}

// syncExecutionScope widens m.executionTickets/m.runTicketIDs to match
// epicName's live RunScope, so a ticket added mid-run via "a"
// (cmdAddToLiveQueue in implement.go, which calls ralphloop.RunScope.Add)
// appears in the Queue tab's list/count/header instead of staying frozen to
// the kickoff snapshot (ticket 06). Only ever grows the set: a ticket already
// recorded stays recorded.
//
// scopeFor returns ok=false once the registry's r.runs entry for epicName is
// gone (loopRegistry.finish, run completed/failed) — the run's own
// r.snapshots entry survives that, though (see loopRegistry doc comment), so
// a Queue tab reattached after the run already finished falls back to
// snapshot's captured ticket set instead of missing it entirely.
func (m *QueueModel) syncExecutionScope(epicName string, snapshot RunSnapshot) {
	if scope, ok := ralphLoopRegistry.scopeFor(epicName); ok {
		for _, epic := range m.epics {
			if epic.Name != epicName {
				continue
			}
			for _, ticket := range epic.Tickets {
				if scope.Contains(ticket, epic) {
					m.recordExecutionTicket(epicName, ticket.Identifier)
				}
			}
			return
		}
		return
	}
	for identifier := range snapshot.Tickets {
		m.recordExecutionTicket(epicName, identifier)
	}
}

// recordExecutionTicket adds epicName/identifier to m.executionTickets and
// m.runTicketIDs if not already present.
func (m *QueueModel) recordExecutionTicket(epicName, identifier string) {
	key := epicName + "/" + identifier
	if m.executionTickets[key] {
		return
	}
	m.executionTickets[key] = true
	m.runTicketIDs[epicName] = append(m.runTicketIDs[epicName], identifier)
}

// markEpicTicketsRunning transitions epicName's captured ticket subset
// (m.runTicketIDs, fixed at kickoff) to running as its ralph-loop starts.
func (m *QueueModel) markEpicTicketsRunning(epicName string) {
	for _, identifier := range m.runTicketIDs[epicName] {
		if path, ok := m.ticketPathFor(epicName, identifier); ok {
			m.setItemStatus(path, queueStatusRunning)
		}
	}
}

// finalizeEpicTicketStatus settles epicName's captured ticket subset once its
// run has stopped: a ticket the registry marked completed lands on done,
// otherwise on errored if the run ended with an error (registry.finish keeps
// the epic's final snapshot around, see loopRegistry.finish's doc comment) —
// left untouched otherwise, e.g. a ticket the concurrency cap never started.
func (m *QueueModel) finalizeEpicTicketStatus(epicName string) {
	snapshot, ok := ralphLoopRegistry.runSnapshot(epicName)
	for _, identifier := range m.runTicketIDs[epicName] {
		path, pathOK := m.ticketPathFor(epicName, identifier)
		if !pathOK {
			continue
		}
		if !ok {
			continue
		}
		if ticket, tok := snapshot.Tickets[identifier]; tok && ticket.Completed {
			m.setItemStatus(path, queueStatusDone)
			continue
		}
		if snapshot.FinalError != "" {
			m.setItemStatus(path, queueStatusErrored)
		}
	}
}

// bindingQueueToggleHideDone is the Queue tab's "tc" chord (ticket 09),
// dispatched through keys.Manager so a key typed right after an unconsumed
// "t" falls through to its own normal action instead of being swallowed
// (ticket 16).
const bindingQueueToggleHideDone keys.BindingID = "toggle-hide-done"

// bindingQueueSelectFirst is the "gg" chord (ticket 11), dispatched through
// keys.Manager since it's a two-key sequence like bindingQueueToggleHideDone
// above.
const bindingQueueSelectFirst keys.BindingID = "select-first"

// bindingQueueHelp is the Queue tab's "?" chord, opening a help modal built
// from this tab's own bindings plus the app-wide extraKeys — mirroring the
// Tickets tab's bindingTicketsHelp.
const bindingQueueHelp keys.BindingID = "help"

// bindingQueueYankSummary/bindingQueueYankFilePath are the Queue tab's
// "yy"/"yf" chords, mirroring the Tickets tab's bindingTicketsYankSummary/
// bindingTicketsYankFilePath (model_keys.go).
const (
	bindingQueueYankSummary  keys.BindingID = "yank-summary"
	bindingQueueYankFilePath keys.BindingID = "yank-file-path"
)

// bindingQueueSelectLast, bindingQueuePreviewBottom, bindingQueueReload,
// bindingQueuePauseResume, bindingQueueClearChecked,
// bindingQueueClearDoneChecked, bindingQueueDelete, and
// bindingQueueSuggestedActions were previously handled by a raw
// msg.String() switch with no keys.Manager entry, so they never appeared in
// the "?" help modal (help.BuildSections only sees this tab's own km, the
// tree's bindings, and the app-wide extraKeys). Registering them here fixes
// that; behavior is unchanged.
const (
	bindingQueueSelectLast       keys.BindingID = "select-last"
	bindingQueuePreviewBottom    keys.BindingID = "preview-bottom"
	bindingQueueReload           keys.BindingID = "reload"
	bindingQueuePauseResume      keys.BindingID = "pause-resume"
	bindingQueueClearChecked     keys.BindingID = "clear-checked"
	bindingQueueClearDoneChecked keys.BindingID = "clear-done-checked"
	bindingQueueDelete           keys.BindingID = "delete"
	bindingQueueSuggestedActions keys.BindingID = "suggested-actions"
)

func newQueueKeysManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingQueueHelp, Seq: []string{"?"}, Categories: []string{"Other"}, Title: "help"},
		{ID: bindingQueueToggleHideDone, Seq: []string{"t", "c"}, Categories: []string{"Navigation"}, Title: "hide completed"},
		{ID: bindingQueueEditInPlace, Seq: []string{"e", "e"}, Categories: []string{"Navigation"}, Title: "edit file"},
		{ID: bindingQueueEditHSplit, Seq: []string{"e", "s"}, Categories: []string{"Navigation"}, Title: "edit file (split)"},
		{ID: bindingQueueEditVSplit, Seq: []string{"e", "v"}, Categories: []string{"Navigation"}, Title: "edit file (vsplit)"},
		{ID: bindingQueueEditTab, Seq: []string{"e", "t"}, Categories: []string{"Navigation"}, Title: "edit file (tab)"},
		{ID: bindingQueueCancelChord, Seq: []string{"e", "esc"}, Categories: []string{}, Title: ""},
		{ID: bindingQueueSelectFirst, Seq: []string{"g", "g"}, Categories: []string{"Navigation"}, Title: "first row"},
		// y-prefix chords
		{ID: bindingQueueYankSummary, Seq: []string{"y", "y"}, Categories: []string{"Yank"}, Title: "yank epic - ticket"},
		{ID: bindingQueueYankFilePath, Seq: []string{"y", "f"}, Categories: []string{"Yank"}, Title: "yank file path"},
		{ID: bindingQueueCancelChord, Seq: []string{"y", "esc"}, Categories: []string{}, Title: ""},
		{ID: bindingQueueSelectLast, Seq: []string{"G"}, Categories: []string{"Navigation"}, Title: "last row"},
		{ID: bindingQueuePreviewBottom, Seq: []string{"b"}, Categories: []string{"Navigation"}, Title: "preview bottom"},
		{ID: bindingQueueReload, Seq: []string{"R"}, Categories: []string{"Other"}, Title: "reload queue"},
		{ID: bindingQueuePauseResume, Seq: []string{"p"}, Categories: []string{"Other"}, Title: "pause/resume queue"},
		{ID: bindingQueueClearChecked, Seq: []string{"C"}, Categories: []string{"Other"}, Title: "clear checked"},
		{ID: bindingQueueClearDoneChecked, Seq: []string{"c"}, Categories: []string{"Other"}, Title: "clear completed checked"},
		{ID: bindingQueueDelete, Seq: []string{"x"}, Categories: []string{"Other"}, Title: "delete"},
		{ID: bindingQueueSuggestedActions, Seq: []string{"m"}, Categories: []string{"Other"}, Title: "suggested actions"},
	})
}

func (m QueueModel) handleQueueKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.implementAgentMenuOpen {
		return m.handleQueueAgentMenuKey(msg)
	}
	match, consumed := m.keys.Process(msg)
	if consumed {
		if match == nil {
			return m, nil // chord in progress
		}
		switch match.ID {
		case bindingQueueHelp:
			m.keys.Reset()
			m.help.Open(m.width, m.height)
		case bindingQueueToggleHideDone:
			m.hideComplete = !m.hideComplete
			m.clampSelected()
		case bindingQueueEditInPlace:
			return m, m.cmdEditSelectedFile(terminalrun.InPlace)
		case bindingQueueEditHSplit:
			return m, m.cmdEditSelectedFile(terminalrun.HSplit)
		case bindingQueueEditVSplit:
			return m, m.cmdEditSelectedFile(terminalrun.VSplit)
		case bindingQueueEditTab:
			return m, m.cmdEditSelectedFile(terminalrun.Tab)
		case bindingQueueCancelChord:
			return m, nil
		case bindingQueueSelectFirst:
			m.selectFirstRow()
		case bindingQueueYankSummary:
			return m, m.yankQueueTicketSummary()
		case bindingQueueYankFilePath:
			return m, m.yankQueueTicketFilePath()
		case bindingQueueSelectLast:
			m.selectLastRow()
		case bindingQueuePreviewBottom:
			m.previewVP.GotoBottom()
		case bindingQueueReload:
			return m, m.cmdLoadQueue()
		case bindingQueuePauseResume:
			hardBudgetPaused := ralphLoopRegistry.isHardLimitPaused()
			softBudgetPaused := ralphLoopRegistry.isSoftLimitPaused()
			var prompt string
			var acceptCmd tea.Cmd
			switch {
			case hardBudgetPaused:
				prompt = budgetHardPauseConfirmPrompt(LiveSpend(), m.settings.Budget.HardLimit)
				acceptCmd = cmdConfirmBudgetOverride()
			case softBudgetPaused:
				prompt = budgetPauseConfirmPrompt(LiveSpend(), m.settings.Budget.SoftLimit)
				acceptCmd = cmdConfirmBudgetOverride()
			case m.paused:
				prompt = "Resume the queue?"
				acceptCmd = cmdConfirmQueueResume()
			default:
				prompt = "Pause the queue?"
				acceptCmd = cmdConfirmQueuePause()
			}
			m.confirm = m.confirm.Open(confirm.Options{Prompt: prompt, AcceptCmd: acceptCmd})
			return m, nil
		case bindingQueueClearChecked:
			if paths := m.checkedPaths(); len(paths) > 0 {
				m.confirm = m.confirm.Open(confirm.Options{
					Prompt:    fmt.Sprintf("Clear all %d queued ticket(s)?", len(paths)),
					AcceptCmd: cmdConfirmQueueClear(paths),
				})
			}
		case bindingQueueClearDoneChecked:
			if paths := m.doneCheckedPaths(); len(paths) > 0 {
				m.confirm = m.confirm.Open(confirm.Options{
					Prompt:    fmt.Sprintf("Clear %d completed ticket(s) from the queue?", len(paths)),
					AcceptCmd: cmdConfirmQueueClear(paths),
				})
			}
		case bindingQueueDelete:
			return m.handleQueueDeleteKey()
		case bindingQueueSuggestedActions:
			return m.handleQueueSuggestedActionsKey()
		}
		return m, nil
	}
	switch msg.String() {
	case "q", "esc":
		return m, nav.Back()
	case "enter":
		// A parked row's "enter" wins over every other meaning below: it
		// resumes that epic (cosmetic wake via Gate.WakeParked, not reattach)
		// rather than launching the checked queue or toggling focus.
		if row, ok := m.selectedQueueRow(); ok {
			if _, parked := ralphLoopRegistry.parkedStalledFor(row.epic.Name); parked {
				ralphLoopRegistry.resumeParked(row.epic.Name)
				return m, nil
			}
		}
		// "enter" launches the checked queue (existing behavior, unrelated to
		// row selection) whenever that's actionable; only when it isn't —
		// nothing checked, or a run's already in flight — does it fall back to
		// the row focus-toggle "l"/"right" also drive (ticket 12), so the two
		// meanings of "enter" never fight over the same press.
		if len(m.runningEpics) == 0 && len(m.pendingEpics) == 0 && len(m.checkedEpicPlans()) > 0 {
			return m.openRunStartModal()
		}
	}

	// Expand-on-already-expanded: tree.Model's own Update reports ExpandNoop
	// on a row that's HasChildren && already Expanded (nothing left to
	// expand); that mutation is discarded (next is dropped rather than
	// assigned back) and focus redirected to the preview pane instead. A
	// leaf row never sets ExpandNoop — it falls through to the tree's own
	// OpenSelected below, which the Queue tab also sends to the preview
	// pane, so leaves land on the same focus-preview outcome as an
	// already-expanded row (the Queue tab's own choice, unlike the sidebar's
	// leaves-are-a-no-op behavior).
	next, cmd, result := m.queueTree.Update(msg)
	if result.ExpandNoop {
		m.focus = focusPreview
		return m, cmd
	}
	m.queueTree = next
	if result.RebuildRequested {
		m.clampSelected()
		if m.search.HasQuery() {
			m.recomputeQueueSearchMatches()
		}
	}
	if result.OpenSelected {
		m.focus = focusPreview
	}
	return m, cmd
}

func (m QueueModel) handleQueueAgentMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "l":
		return m.startCheckedEpic(ralphloop.AgentClaude)
	case "o":
		return m.startCheckedEpic(ralphloop.AgentCodex)
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
	value := m.implementAgentMenu.Items[m.implementAgentMenu.Cursor].Value
	if value == "cancel" {
		m.implementAgentMenuOpen = false
		return m, nil
	}
	return m.startCheckedEpic(ralphloop.AgentKind(value))
}

// newRunStartAgentMenu is the Queue tab's run-start pick-list, offered only
// when openRunStartModal finds more than one available agent — a trailing
// Cancel item alongside Claude/Codex, since this modal (unlike the Tickets
// tab's dead agent-picker it replaces) has no separate confirm step for
// picking to fall back on.
func newRunStartAgentMenu() components.MenuState {
	return components.MenuState{
		Items: []components.MenuItem{
			{Label: "l  Claude", Value: string(ralphloop.AgentClaude)},
			{Label: "o  Codex", Value: string(ralphloop.AgentCodex)},
			{Label: "Cancel", Value: "cancel"},
		},
		Cursor: 0,
	}
}

// openRunStartModal implements ticket 09a's unified run-start modal: a
// banner (the subscription safety-check line plus configured budget limits)
// above an action area scoped to whichever agents are actually usable on
// this machine right now (see availableAgents — Codex's availability is
// rechecked fresh, uncached, on every open). Exactly one available agent
// collapses the action area to a plain Yes/No confirmation; more than one
// opens the pick-an-agent list (plus Cancel), where picking confirms
// directly; zero is a defensive, currently-unreachable case (Claude has no
// availability check) that never opens a modal at all.
func (m QueueModel) openRunStartModal() (tea.Model, tea.Cmd) {
	agents := availableAgents()
	banner := runStartBannerText(m.settings.Budget, m.settings.Subscription)
	switch len(agents) {
	case 0:
		return m, notify.Info("no agent is available to run this epic")
	case 1:
		prompt := fmt.Sprintf("Start the checked selection with %s?", agentDisplayName(agents[0]))
		if banner != "" {
			prompt = banner + "\n\n" + prompt
		}
		m.confirm = m.confirm.Open(confirm.Options{
			Prompt:    prompt,
			AcceptCmd: cmdConfirmRunStart(agents[0]),
		})
		return m, nil
	default:
		m.implementAgentMenu = newRunStartAgentMenu()
		m.implementAgentMenuOpen = true
		return m, nil
	}
}

// runStartConfirmedMsg carries the run-start modal's Yes/No confirmation
// acceptance: agent is captured when the modal opened (mirroring
// queueClearConfirmedMsg's same capture-at-open-time approach).
type runStartConfirmedMsg struct {
	agent ralphloop.AgentKind
}

func cmdConfirmRunStart(agent ralphloop.AgentKind) tea.Cmd {
	return func() tea.Msg {
		return runStartConfirmedMsg{agent: agent}
	}
}

// handleRunStartConfirmed applies runStartConfirmedMsg by launching the
// checked selection with the confirmed agent, same as picking an agent
// directly from the pick-list branch.
func (m QueueModel) handleRunStartConfirmed(msg runStartConfirmedMsg) (tea.Model, tea.Cmd) {
	return m.startCheckedEpic(msg.agent)
}

func (m QueueModel) startCheckedEpic(agent ralphloop.AgentKind) (tea.Model, tea.Cmd) {
	m.pendingEpics = m.checkedEpicPlans()
	if len(m.pendingEpics) == 0 {
		m.implementAgentMenuOpen = false
		return m, nil
	}
	m.implementAgentMenuOpen = false
	m.executionStartedAt = time.Time{}
	m.executionCompletedAt = time.Time{}
	m.executionTickets = map[string]bool{}
	m.runTicketIDs = map[string][]string{}
	for _, plan := range m.pendingEpics {
		m.runTicketIDs[plan.epic.Name] = append([]string(nil), plan.ticketIDs...)
		for _, ticketID := range plan.ticketIDs {
			m.executionTickets[plan.epic.Name+"/"+ticketID] = true
		}
	}
	m.runningAgent = agent
	return m, m.startAvailableEpics()
}

type checkedEpicPlan struct {
	epic tickets.Epic
	// ticketIDs is the checked-set snapshot, always used for this Model's own
	// progress accounting (executionTickets, runTicketIDs) regardless of
	// dynamic.
	ticketIDs []string
	// dynamic reports whether ticketIDs covers every currently-eligible
	// (non-done) ticket in epic, in which case startAvailableEpics launches
	// with an empty RunOptions.TicketIDs so the run stays dynamic (rescans the
	// epic on disk every claim, per ralphloop.ResolveRunScope) instead of
	// freezing scope to this snapshot — matching the single-epic "i" launch
	// path. A genuine subset (some eligible ticket deliberately left
	// unchecked) still freezes to exactly ticketIDs.
	dynamic bool
	done    int
	ordinal uint64
	ordered bool
}

func (m QueueModel) checkedEpicPlans() []checkedEpicPlan {
	return checkedEpicPlansFor(m.epics, m.checked, m.checkOrder)
}

// checkedEpicPlansFor is checkedEpicPlans' free-function body, shared with the
// Tickets tab's drain-then-replace combo (handleDrainReplaceKey in
// drain_replace.go) so both callers build the same launch plan from whichever
// checked/checkOrder pair they hold.
func checkedEpicPlansFor(epics []tickets.Epic, checked map[string]bool, checkOrder map[string]uint64) []checkedEpicPlan {
	plans := make([]checkedEpicPlan, 0, len(epics))
	for _, epic := range epics {
		var ticketIDs []string
		done := 0
		eligible := 0
		var ordinal uint64
		ordered := false
		for _, idx := range sortedTicketIndexes(epic) {
			ticket := epic.Tickets[idx]
			if epic.RenderedStatus(ticket) != tickets.StatusDone {
				eligible++
			}
			if !checked[ticket.Path] {
				continue
			}
			ticketIDs = append(ticketIDs, ticket.Identifier)
			if epic.RenderedStatus(ticket) == tickets.StatusDone {
				done++
			}
			if checkedAt, ok := checkOrder[ticket.Path]; ok && (!ordered || checkedAt < ordinal) {
				ordinal, ordered = checkedAt, true
			}
		}
		if len(ticketIDs) > 0 {
			plans = append(plans, checkedEpicPlan{
				epic: epic, ticketIDs: ticketIDs, dynamic: len(ticketIDs) == eligible,
				done: done, ordinal: ordinal, ordered: ordered,
			})
		}
	}
	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].ordered != plans[j].ordered {
			return plans[i].ordered
		}
		if plans[i].ordered && plans[i].ordinal != plans[j].ordinal {
			return plans[i].ordinal < plans[j].ordinal
		}
		return plans[i].epic.Name < plans[j].epic.Name
	})
	return plans
}

func (m *QueueModel) startAvailableEpics() tea.Cmd {
	count := min(ralphLoopRegistry.availableSlots(), len(m.pendingEpics))
	cmds := make([]tea.Cmd, 0, count)
	for _, plan := range m.pendingEpics[:count] {
		runTicketIDs := plan.ticketIDs
		if plan.dynamic {
			runTicketIDs = nil
		}
		cmds = append(cmds, cmdStartImplement(
			m.worktreeRoot, plan.epic.Name, m.runningAgent, plan.done, len(plan.ticketIDs),
			m.settings.MaxConcurrentTicketsPerEpic(), runTicketIDs, m.settings.Notifications,
			m.settings.ImplementSkill(), m.settings.ResolvedAgents(),
		))
	}
	m.pendingEpics = m.pendingEpics[count:]
	if len(cmds) == 1 {
		return cmds[0]
	}
	return func() tea.Msg {
		messages := make(tea.BatchMsg, 0, len(cmds))
		for _, cmd := range cmds {
			msg := cmd()
			messages = append(messages, func() tea.Msg { return msg })
		}
		return messages
	}
}

// selectFirstRow/selectLastRow implement "gg"/"G": jump the queue selection
// to the first/last row, mirroring the Tickets tab's own selectFirstRow/
// selectLastRow (model_keys.go).
func (m *QueueModel) selectFirstRow() {
	if len(m.queueTree.Entries()) == 0 {
		return
	}
	m.queueTree.SetSelectedIndex(0)
	m.queueTree.SkipUnselectable(1)
}

func (m *QueueModel) selectLastRow() {
	n := len(m.queueTree.Entries())
	if n == 0 {
		return
	}
	m.queueTree.SetSelectedIndex(n - 1)
	m.queueTree.SkipUnselectable(-1)
}

// clampSelected rebuilds the queue tree's entries from the current
// epics/hideComplete/collapse state — SetEntries re-clamps selection to the
// new entry count.
func (m *QueueModel) clampSelected() {
	m.queueTree.SetEntries(m.buildQueueEntries())
	m.queueTree.SkipUnselectable(1)
}

// queueViewportHeight is the queue panel's visible body line count, matching
// ui.RenderPanel's own bodyH math (PaddingY: 0, minus the header row) — see
// View()'s split sizing — so the windowing done here lines up with what
// RenderPanel actually paints. Splits its height the same way View() does
// (splitPanelHeight), since a stacked (narrow-terminal) layout shares the
// available height with the preview pane below it.
func (m QueueModel) queueViewportHeight() int {
	sidebarH, _ := splitPanelHeight(m.width, m.contentHeight())
	return max(sidebarH-1, 0)
}
