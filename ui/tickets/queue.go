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
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/terminalrun"
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

	selected     int
	scrollOffset int

	implementAgentMenuOpen bool
	implementAgentMenu     components.MenuState
	pendingEpics           []checkedEpicPlan
	runningEpics           map[string]bool
	runningAgent           ralphloop.AgentKind
	paused                 bool

	// search backs "/"-triggered filtering over rows(), mirroring the Tickets
	// tab's own m.search (see ui/tickets/search.go).
	search search.Model

	// confirm backs the "C"/"c" clear keymaps and the "x" cascade-delete keymap
	// (handleQueueKey) — the Queue tab is read-only for selection (ticket 08),
	// so this is the only modal this tab opens outside the agent-picker menu.
	confirm confirm.Model

	// hideComplete backs the "tc" chord (ticket 09): when true, rows()/
	// rowsAndPlanErrors() omit StatusDone tickets from the rendered row list,
	// independent of the "c"/"C" clear keymaps (which mutate the queue store,
	// not visibility) and independent of epicWaves' plan validation, which
	// must keep considering hidden-but-still-queued tickets.
	hideComplete bool
	// keys dispatches the "tc" chord above through ui/keys.Manager so a key
	// typed right after an unconsumed "t" falls through to its own normal
	// action instead of being swallowed (ticket 16).
	keys keys.Manager
	help help.Model
	// collapsedQueueTickets is the Queue tab's counterpart to the Tickets
	// tab's collapsedTickets (ticket 09), keyed by Ticket.Path, true for a
	// ticket whose children (Parent/Children, ticket 03) are hidden in
	// rows()/rowsAndPlanErrors() (ticket 10). Every ticket with children
	// starts expanded — no default-collapse pass.
	collapsedQueueTickets map[string]bool

	// previewFocus backs the preview panel's scroll/search machinery, shared
	// with the Tickets tab (see preview_focus.go) — ticket 11 gave the Queue
	// tab real scroll/search instead of the old truncate-only preview, and
	// ticket 12 wires its promoted focus field up to "l"/"right"/"enter" and
	// "h"/"left"/"esc" (see queueFocusPreviewOrExpand/handleQueuePreviewKey in
	// queue_preview.go), mirroring the Tickets tab's own focus-toggle.
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
		implementAgentMenu: newImplementAgentMenu(),
		runningEpics:       map[string]bool{},
		paused:             ralphLoopRegistry.isPaused(),
		confirm:            confirm.New(),
		search:             search.NewModel(),
		keys:               km,
		help:               help.NewModel(help.BuildSections(km, extraKeys)),
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
	epics []tickets.Epic
	err   error
}

func (m QueueModel) cmdLoadQueue() tea.Cmd {
	scratchDir := scratchDirFor(m.worktreeRoot)
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		return queueEpicsLoadedMsg{epics: epics, err: err}
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
		m.ensureQueueVisible()
		return m, nil
	case queueEpicsLoadedMsg:
		if err := autoQueueSplitChildren(m.epics, msg.epics, m.queueStore); err != nil {
			return m, notify.Error("save queue: " + err.Error())
		}
		if m.queueStore != nil {
			snapshot := m.queueStore.Snapshot()
			m.checked = snapshot.Checked
			m.checkOrder = snapshot.Order
			m.queueStatus = snapshot.Status
		}
		m.loaded = true
		m.epics = msg.epics
		m.candidates = make(map[string]bool, len(m.checked))
		for path := range m.checked {
			m.candidates[path] = true
		}
		if m.search.HasQuery() {
			m.recomputeQueueSearchMatches()
		}
		m.clampSelected()
		var autoRefreshCmd tea.Cmd
		if !m.autoRefreshStarted {
			m.autoRefreshStarted = true
			autoRefreshCmd = cmdAutoRefresh()
		}
		return m, autoRefreshCmd

	case autoRefreshMsg:
		return m, tea.Batch(m.cmdLoadQueue(), cmdAutoRefresh())
	case implementStartedMsg:
		if m.executionStartedAt.IsZero() {
			m.executionStartedAt = m.now()
		}
		m.runningEpics[msg.epicName] = true
		m.markEpicTicketsRunning(msg.epicName)
		m.syncRunSnapshot(msg.epicName)
		return m, tea.Batch(cmdPollImplement(msg.epicName), m.implementSpinner.Tick)
	case implementPollMsg:
		m.syncRunSnapshot(msg.epicName)
		if ralphLoopRegistry.isRunningEpic(msg.epicName) {
			return m, cmdPollImplement(msg.epicName)
		}
		m.finalizeEpicTicketStatus(msg.epicName)
		delete(m.runningEpics, msg.epicName)
		delete(m.live, msg.epicName)
		executionComplete := !m.executionStartedAt.IsZero() && len(m.runningEpics) == 0 && len(m.pendingEpics) == 0
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
		return m.handleQueueMouseWheel(msg)
	case queueClearConfirmedMsg:
		return m.handleQueueClearConfirmed(msg)
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
		if m.confirm.IsOpen {
			return m.handleQueueConfirmMouseUpdate(msg)
		}
		return m.handleQueueMouseClick(msg)
	case cascadeDeleteConfirmedMsg:
		return m.handleCascadeDeleteConfirmed(msg)
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

// handleQueueMouseWheel scrolls the queue viewport without moving selection,
// mirroring ui/log's handleMouseWheel.
func (m QueueModel) handleQueueMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	dir, ok := ui.WheelDirection(msg)
	if !ok {
		return m, nil
	}
	if m.focus == focusPreview {
		var cmd tea.Cmd
		m.previewVP, cmd = m.previewVP.Update(msg)
		return m, cmd
	}
	m.scrollOffset += dir * ui.WheelScrollLines
	m.clampScrollOffset()
	return m, nil
}

// handleQueueMouseClick selects the row under the click without triggering
// any secondary action (no confirm, no checkbox toggle) — the Queue panel
// renders full-width with no header above it, so only Y needs bounds
// checking.
func (m QueueModel) handleQueueMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	bodyLine := mouse.Y - 1
	if bodyLine < 0 {
		return m, nil
	}
	line := bodyLine + m.scrollOffset
	_, offsets, heights := m.buildQueueLines()
	for i, offset := range offsets {
		if line >= offset && line < offset+heights[i] {
			m.selected = i
			m.ensureQueueVisible()
			return m, nil
		}
	}
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

func (m QueueModel) handleQueueSync(msg queueSyncMsg) (tea.Model, tea.Cmd) {
	m.paused = msg.paused
	byName := make(map[string]RunSnapshot, len(msg.snapshots))
	for _, s := range msg.snapshots {
		byName[s.EpicName] = s
	}
	var cmds []tea.Cmd
	for _, plan := range m.checkedEpicPlans() {
		name := plan.epic.Name
		snapshot, ok := byName[name]
		if !ok {
			continue
		}
		m.syncRunSnapshot(name)
		if !snapshot.StartedAt.IsZero() && (m.executionStartedAt.IsZero() || snapshot.StartedAt.Before(m.executionStartedAt)) {
			m.executionStartedAt = snapshot.StartedAt
		}
		wasRunning := m.runningEpics[name]
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
	if len(m.executionTickets) == 0 {
		for _, plan := range m.checkedEpicPlans() {
			if _, ok := byName[plan.epic.Name]; !ok {
				continue
			}
			m.runTicketIDs[plan.epic.Name] = append([]string(nil), plan.ticketIDs...)
			for _, ticketID := range plan.ticketIDs {
				m.executionTickets[plan.epic.Name+"/"+ticketID] = true
			}
		}
	}
	cmds = append(cmds, m.startAvailableEpics())
	if !m.executionStartedAt.IsZero() && m.executionCompletedAt.IsZero() && len(m.runningEpics) == 0 && len(m.pendingEpics) == 0 {
		m.executionCompletedAt = m.now()
		cmds = append(cmds, m.cmdLoadQueue())
	}
	return m, tea.Batch(cmds...)
}

func (m *QueueModel) syncRunSnapshot(epicName string) {
	snapshot, ok := ralphLoopRegistry.runSnapshot(epicName)
	if !ok {
		return
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
		}
		return m, nil
	}
	switch msg.String() {
	case "q", "esc":
		return m, nav.Back()
	case "j", "down":
		m.moveSelection(1)
	case "k", "up":
		m.moveSelection(-1)
	case "h", "left":
		m.collapseSelectedQueueRow()
	case "l", "right":
		if m.queueFocusPreviewOrExpand() {
			return m, nil
		}
		m.expandSelectedQueueRow()
	case "G":
		m.selectLastRow()
	case "b":
		m.previewVP.GotoBottom()
	case "ctrl+d":
		m.moveSelection(list.DefaultScroll)
	case "ctrl+u":
		m.moveSelection(-list.DefaultScroll)
	case "R":
		return m, m.cmdLoadQueue()
	case "p":
		if m.paused {
			ralphLoopRegistry.resume()
			m.paused = false
			return m, tea.Batch(notify.Success("queue resumed"), m.startAvailableEpics())
		}
		ralphLoopRegistry.pause()
		m.paused = true
		return m, notify.Info("queue paused")
	case "C":
		if paths := m.checkedPaths(); len(paths) > 0 {
			m.confirm = m.confirm.Open(confirm.Options{
				Prompt:    fmt.Sprintf("Clear all %d queued ticket(s)?", len(paths)),
				AcceptCmd: cmdConfirmQueueClear(paths),
			})
		}
	case "c":
		if paths := m.doneCheckedPaths(); len(paths) > 0 {
			m.confirm = m.confirm.Open(confirm.Options{
				Prompt:    fmt.Sprintf("Clear %d completed ticket(s) from the queue?", len(paths)),
				AcceptCmd: cmdConfirmQueueClear(paths),
			})
		}
	case "x":
		return m.handleQueueDeleteKey()
	case "enter":
		// "enter" launches the checked queue (existing behavior, unrelated to
		// row selection) whenever that's actionable; only when it isn't —
		// nothing checked, or a run's already in flight — does it fall back to
		// the row focus-toggle "l"/"right" also drive (ticket 12), so the two
		// meanings of "enter" never fight over the same press.
		if len(m.runningEpics) == 0 && len(m.pendingEpics) == 0 && len(m.checkedEpicPlans()) > 0 {
			m.implementAgentMenu = newImplementAgentMenu()
			m.implementAgentMenuOpen = true
			return m, nil
		}
		if m.queueFocusPreviewOrExpand() {
			return m, nil
		}
		m.expandSelectedQueueRow()
	}
	return m, nil
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
	agent := ralphloop.AgentKind(m.implementAgentMenu.Items[m.implementAgentMenu.Cursor].Value)
	return m.startCheckedEpic(agent)
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
	epic      tickets.Epic
	ticketIDs []string
	done      int
	ordinal   uint64
	ordered   bool
}

func (m QueueModel) checkedEpicPlans() []checkedEpicPlan {
	plans := make([]checkedEpicPlan, 0, len(m.epics))
	for _, epic := range m.epics {
		var ticketIDs []string
		done := 0
		var ordinal uint64
		ordered := false
		for _, idx := range sortedTicketIndexes(epic) {
			ticket := epic.Tickets[idx]
			if !m.checked[ticket.Path] {
				continue
			}
			ticketIDs = append(ticketIDs, ticket.Identifier)
			if epic.RenderedStatus(ticket) == tickets.StatusDone {
				done++
			}
			if checkedAt, ok := m.checkOrder[ticket.Path]; ok && (!ordered || checkedAt < ordinal) {
				ordinal, ordered = checkedAt, true
			}
		}
		if len(ticketIDs) > 0 {
			plans = append(plans, checkedEpicPlan{epic: epic, ticketIDs: ticketIDs, done: done, ordinal: ordinal, ordered: ordered})
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
		cmds = append(cmds, cmdStartImplement(
			m.worktreeRoot, plan.epic.Name, m.runningAgent, plan.done, len(plan.ticketIDs),
			m.settings.MaxConcurrentTicketsPerEpic(), plan.ticketIDs, m.settings.Notifications,
			m.settings.ImplementSkill(),
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

// collapseSelectedQueueRow handles "h"/"left": on a row with children that's
// currently expanded it collapses that row's own children, mirroring the
// Tickets tab's collapseSelectedEpic (ticket 09) one level down since the
// Queue tab has no epic-level collapse of its own. On any other row (a leaf,
// or one already collapsed) with a nested parent it jumps selection up to
// that parent row instead.
func (m *QueueModel) collapseSelectedQueueRow() {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return
	}
	r := rows[m.selected]
	if r.hasChildren && r.expanded {
		m.setCollapsedQueueTicket(r.ticket.Path, true)
		return
	}
	if r.parentPath != "" {
		m.jumpToQueueTicket(r.parentPath)
	}
}

// expandSelectedQueueRow handles "l"/"right": on a collapsed row with
// children it expands that row's own children, mirroring the Tickets tab's
// expandSelectedEpic (ticket 09) one level down.
func (m *QueueModel) expandSelectedQueueRow() {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return
	}
	r := rows[m.selected]
	if r.hasChildren && !r.expanded {
		m.setCollapsedQueueTicket(r.ticket.Path, false)
	}
}

// jumpToQueueTicket moves the selection to the row for the ticket at path,
// collapseSelectedQueueRow's counterpart to the Tickets tab's jumpToTicket.
// The target is always present in rows(): a child row only ever appears once
// every one of its ancestors is expanded.
func (m *QueueModel) jumpToQueueTicket(path string) {
	for i, r := range m.rows() {
		if r.ticket.Path == path {
			m.selected = i
			m.ensureQueueVisible()
			return
		}
	}
}

// setCollapsedQueueTicket is the Queue tab's counterpart to the Tickets
// tab's setCollapsedTicket (ticket 09), keyed by Ticket.Path since it's read
// from collapsedQueueTickets by ui/tree's entry-builder inside
// queueRowsForEpic.
func (m *QueueModel) setCollapsedQueueTicket(path string, collapsed bool) {
	if m.collapsedQueueTickets == nil {
		m.collapsedQueueTickets = map[string]bool{}
	}
	if collapsed {
		m.collapsedQueueTickets[path] = true
	} else {
		delete(m.collapsedQueueTickets, path)
	}
	if m.search.HasQuery() {
		m.recomputeQueueSearchMatches()
	}
	m.clampSelected()
}

// selectFirstRow/selectLastRow implement "gg"/"G": jump the queue selection
// to the first/last row, mirroring the Tickets tab's own selectFirstRow/
// selectLastRow (model_keys.go).
func (m *QueueModel) selectFirstRow() {
	if len(m.rows()) == 0 {
		return
	}
	m.selected = 0
	m.ensureQueueVisible()
}

func (m *QueueModel) selectLastRow() {
	n := len(m.rows())
	if n == 0 {
		return
	}
	m.selected = n - 1
	m.ensureQueueVisible()
}

func (m *QueueModel) moveSelection(delta int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(rows) {
		m.selected = len(rows) - 1
	}
	m.ensureQueueVisible()
}

func (m *QueueModel) clampSelected() {
	n := len(m.rows())
	switch {
	case n == 0:
		m.selected = 0
	case m.selected >= n:
		m.selected = n - 1
	case m.selected < 0:
		m.selected = 0
	}
	m.ensureQueueVisible()
}

// queueViewportHeight is the queue panel's visible body line count, matching
// ui.RenderPanel's own bodyH math (PaddingY: 0, minus the header row) — see
// View()'s split sizing — so the windowing done here lines up with what
// RenderPanel actually paints. Splits its height the same way View() does
// (splitPanelHeight), since a stacked (narrow-terminal) layout shares the
// available height with the preview pane below it.
func (m QueueModel) queueViewportHeight() int {
	height := max(m.height-1, 1)
	sidebarH, _ := splitPanelHeight(m.width, height)
	return max(sidebarH-1, 0)
}

// ensureQueueVisible adjusts scrollOffset minimally so the selected row's
// lines stay within the queue panel's visible window, mirroring the Tickets
// tab's ensureSidebarVisible (model.go) — needed because a row can be one or
// two physical lines depending on its live/done status.
func (m *QueueModel) ensureQueueVisible() {
	viewportH := m.queueViewportHeight()
	line, rowHeight, ok := m.queueLineForSelected()
	if ok {
		if line < m.scrollOffset {
			m.scrollOffset = line
		}
		lastLine := line + rowHeight - 1
		if viewportH > 0 && lastLine >= m.scrollOffset+viewportH {
			m.scrollOffset = lastLine - viewportH + 1
		}
	}
	m.clampScrollOffset()
}

func (m *QueueModel) clampScrollOffset() {
	total := len(m.queueLines())
	viewportH := m.queueViewportHeight()
	m.scrollOffset = ui.ClampScrollOffset(m.scrollOffset, total, viewportH)
}
