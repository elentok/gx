package tickets

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/confirm"
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
}

func NewQueueModel(worktreeRoot string, settings ui.Settings, checked map[string]bool, orders ...map[string]uint64) QueueModel {
	if checked == nil {
		checked = map[string]bool{}
	}
	checkOrder := map[string]uint64{}
	if len(orders) > 0 && orders[0] != nil {
		checkOrder = orders[0]
	}
	sp := spinner.New()
	sp.Spinner = TicketProgressSpinner
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
		keys:               newQueueKeysManager(),
	}
}

func NewQueueModelWithStore(worktreeRoot string, settings ui.Settings, store *QueueStore) QueueModel {
	snapshot := store.Snapshot()
	m := NewQueueModel(worktreeRoot, settings, snapshot.Checked, snapshot.Order)
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
	scratchDir := filepath.Join(m.worktreeRoot, ".scratch")
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		return queueEpicsLoadedMsg{epics: epics, err: err}
	}
}

func (m QueueModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.confirm.IsOpen {
			return m.handleQueueConfirmUpdate(msg)
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

// e-prefix chords: edit the selected ticket's underlying file, mirroring the
// Tickets tab's own edit-file chord (model_keys.go) via the shared
// editTicketFile helper (model_runtime.go).
const (
	bindingQueueEditInPlace keys.BindingID = "edit"
	bindingQueueEditHSplit  keys.BindingID = "edit-hsplit"
	bindingQueueEditVSplit  keys.BindingID = "edit-vsplit"
	bindingQueueEditTab     keys.BindingID = "edit-tab"
	bindingQueueCancelChord keys.BindingID = "cancel-chord"
)

func newQueueKeysManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingQueueToggleHideDone, Seq: []string{"t", "c"}, Categories: []string{"Navigation"}, Title: "hide completed"},
		{ID: bindingQueueEditInPlace, Seq: []string{"e", "e"}, Categories: []string{"Navigation"}, Title: "edit file"},
		{ID: bindingQueueEditHSplit, Seq: []string{"e", "s"}, Categories: []string{"Navigation"}, Title: "edit file (split)"},
		{ID: bindingQueueEditVSplit, Seq: []string{"e", "v"}, Categories: []string{"Navigation"}, Title: "edit file (vsplit)"},
		{ID: bindingQueueEditTab, Seq: []string{"e", "t"}, Categories: []string{"Navigation"}, Title: "edit file (tab)"},
		{ID: bindingQueueCancelChord, Seq: []string{"e", "esc"}, Categories: []string{}, Title: ""},
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
		if len(m.runningEpics) == 0 && len(m.pendingEpics) == 0 {
			if len(m.checkedEpicPlans()) > 0 {
				m.implementAgentMenu = newImplementAgentMenu()
				m.implementAgentMenuOpen = true
			}
		}
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
			m.settings.MaxConcurrentTicketsPerEpic(), plan.ticketIDs, m.settings.Notifications.Telegram,
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

// cmdEditSelectedFile opens the selected row's ticket file for editing,
// mirroring the Tickets tab's Model.cmdEditSelectedFile — the Queue tab only
// ever selects tickets (no epic rows), so there's no map.md case to handle.
func (m QueueModel) cmdEditSelectedFile(splitType terminalrun.SplitType) tea.Cmd {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return notify.Warning("nothing selected")
	}
	return editTicketFile(m.worktreeRoot, m.settings, rows[m.selected].ticket.Path, splitType)
}

func (m QueueModel) handleEditFileFinished(msg editFileFinishedMsg) (QueueModel, tea.Cmd) {
	return m, editFileFinishedCmd(msg)
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

func (m QueueModel) View() tea.View {
	if m.queueStore != nil {
		snapshot := m.queueStore.Snapshot()
		m.checked = snapshot.Checked
		m.checkOrder = snapshot.Order
		m.queueStatus = snapshot.Status
	}
	if !m.ready {
		return ui.NewMainView("\n  Initializing…")
	}
	if m.implementAgentMenuOpen {
		plans := m.checkedEpicPlans()
		return ui.NewMainView(renderImplementAgentMenu(
			fmt.Sprintf("Choose the agent for %d checked epic(s):", len(plans)),
			m.implementAgentMenu,
		))
	}
	viewportH := m.queueViewportHeight()
	height := max(m.height-1, 1)
	sidebarW, previewW := splitPanelWidth(m.width)
	sidebarH, previewH := splitPanelHeight(m.width, height)
	lines := ui.AppendScrollbar(m.queueVisibleLines(viewportH), sidebarW-2, len(m.queueLines()), viewportH, m.scrollOffset)

	queueView := ui.RenderPanel(ui.PanelOptionsFor(
		sidebarW, sidebarH, m.queueHeaderTitle(), "", lines, true, ui.ColorBlue, nil, true,
	))
	previewView := ui.RenderPanel(ui.PanelOptionsFor(
		previewW, previewH, "Preview", "", m.queuePreviewLines(previewW, previewH), false, ui.ColorBlue, nil, false,
	))

	var content string
	if useStackedLayout(m.width) {
		seam := ui.RenderSeamRow(sidebarW, ui.SeamColor)
		content = lipgloss.JoinVertical(lipgloss.Left, queueView, seam, previewView)
	} else {
		seam := ui.RenderSeamColumn(sidebarH, ui.SeamColor)
		content = lipgloss.JoinHorizontal(lipgloss.Top, queueView, seam, previewView)
	}

	if m.confirm.IsOpen {
		content = ui.OverlayCenter(content, m.confirm.View(m.width), m.width, m.height)
	}
	if m.search.Mode() == search.SearchModeInput {
		overlayW := m.searchOverlayWidth()
		activeSearch := m.search
		activeSearch.SetWidth(overlayW)
		overlay := activeSearch.View()
		y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
		content = ui.OverlayBottomCenter(content, overlay, m.width, y)
	}
	return ui.NewMainView(content)
}

// queueVisibleLines windows queueLines() to a single viewportH-line scroll
// position at m.scrollOffset, mirroring Model.sidebarVisibleLines.
func (m QueueModel) queueVisibleLines(viewportH int) []string {
	lines := m.queueLines()
	start := min(m.scrollOffset, len(lines))
	end := min(start+viewportH, len(lines))
	return lines[start:end]
}

// queueRunStateKind classifies the queue's run state for header rendering.
// queueHeaderTitle and queueHeaderBodyLines both switch on the same
// queueRunState() call so they can't independently re-derive (and silently
// diverge on) what state the queue is in.
type queueRunStateKind int

const (
	queueRunIdle queueRunStateKind = iota
	queueRunRunning
	queueRunPaused
	queueRunCompleted
)

// queueRunState classifies the queue's current run state. Paused only wins
// over idle once a run has actually captured a ticket scope
// (m.checkedProgress total > 0) — m.paused alone can be set by the bare `p`
// key with no run-state guard, so a queue that was never started must still
// classify as idle even while globally paused.
func (m QueueModel) queueRunState() queueRunStateKind {
	if !m.executionCompletedAt.IsZero() {
		done, total := m.completedExecutionProgress()
		if total > 0 && done == total {
			return queueRunCompleted
		}
	}
	if m.paused {
		if _, total := m.checkedProgress(); total > 0 {
			return queueRunPaused
		}
	}
	if len(m.runningEpics) > 0 {
		return queueRunRunning
	}
	return queueRunIdle
}

// queueHeaderTitle and queueHeaderBodyLines together implement the Option B
// header redesign (see .scratch/tickets-queue-batch3/issues/assets/08-header-prototype.md):
// the title always encodes run state, and the body carries at most one
// state-specific line instead of the old always-present banner row.
func (m QueueModel) queueHeaderTitle() string {
	switch m.queueRunState() {
	case queueRunCompleted:
		elapsed := int(m.executionCompletedAt.Sub(m.executionStartedAt).Seconds())
		return fmt.Sprintf("Queue · done, took %s", formatElapsed(elapsed))
	case queueRunPaused:
		done, total := m.checkedProgress()
		return fmt.Sprintf("Queue · paused (%d of %d done)", done, total)
	case queueRunRunning:
		done, total := m.checkedProgress()
		glyph := strings.TrimRight(m.implementSpinner.View(), " ")
		return fmt.Sprintf("Queue · %d of %d done · %s implementing...", done, total, glyph)
	default:
		return "Queue"
	}
}

func (m QueueModel) queueHeaderBodyLines() []string {
	switch m.queueRunState() {
	case queueRunCompleted:
		total, average, maximum := m.completedContextMetrics()
		return []string{fmt.Sprintf(
			"context windows: total %s, avg %s, max %s",
			formatTokenCount(total), formatTokenCount(average), formatTokenCount(maximum),
		)}
	case queueRunPaused:
		if len(m.runningEpics) > 0 {
			return []string{"Queue paused — in-flight iterations will finish"}
		}
		return nil
	default:
		return nil
	}
}

func (m QueueModel) completedContextMetrics() (total, average, maximum int) {
	count := 0
	for _, epic := range m.epics {
		for _, ticket := range epic.Tickets {
			if !m.executionTickets[epic.Name+"/"+ticket.Identifier] {
				continue
			}
			count++
			total += ticket.ActualContextWindow
			maximum = max(maximum, ticket.ActualContextWindow)
		}
	}
	if count > 0 {
		average = total / count
	}
	return total, average, maximum
}

func (m QueueModel) completedExecutionProgress() (done, total int) {
	for _, epic := range m.epics {
		for _, ticket := range epic.Tickets {
			if !m.executionTickets[epic.Name+"/"+ticket.Identifier] {
				continue
			}
			total++
			if epic.RenderedStatus(ticket) == tickets.StatusDone {
				done++
			}
		}
	}
	return done, total
}

// checkedProgress reports the active run's done/total ticket counts, scoped
// to m.executionTickets — the run's captured selection at kickoff — rather
// than the live m.checked set, so editing the checked selection while a run
// is active doesn't rewrite that run's progress totals (ticket 20).
func (m QueueModel) checkedProgress() (int, int) {
	done := 0
	total := 0
	for _, epic := range m.epics {
		for _, ticket := range epic.Tickets {
			if !m.executionTickets[epic.Name+"/"+ticket.Identifier] {
				continue
			}
			total++
			if epic.RenderedStatus(ticket) == tickets.StatusDone || m.queueStatus[ticket.Path] == queueStatusDone {
				done++
			}
		}
	}
	return done, total
}

func (m QueueModel) queueLines() []string {
	lines, _, _ := m.buildQueueLines()
	return lines
}

// queueLineForSelected returns the selected row's line index and physical
// height within queueLines()'s output, mirroring Model.sidebarLineForSelected
// — needed since a row is one or two physical lines depending on its
// live/done status (renderQueueTicketRow).
func (m QueueModel) queueLineForSelected() (line, height int, ok bool) {
	_, offsets, heights := m.buildQueueLines()
	if m.selected < 0 || m.selected >= len(offsets) {
		return 0, 0, false
	}
	return offsets[m.selected], heights[m.selected], true
}

// buildQueueLines renders every candidate ticket, grouped by epic and
// ordered in plan order (rowsAndPlanErrors), as the same two-line status rows
// the Tickets tab renders for its own tickets (renderQueueTicketRow) — no
// "parallel"/"then" wave grouping (ticket 25). offsets/heights are aligned to
// rowsAndPlanErrors' row order so selection/scroll math can find any row's
// position in lines without re-deriving the rendering.
func (m QueueModel) buildQueueLines() (lines []string, offsets []int, heights []int) {
	if !m.loaded {
		return []string{ui.StyleDim.Render("  loading…")}, nil, nil
	}

	for _, line := range m.queueHeaderBodyLines() {
		lines = append(lines, "  "+ui.StyleHint.Render(line))
	}

	rows, planErrs := m.rowsAndPlanErrors()
	if len(rows) == 0 {
		lines = append(lines, ui.StyleMuted.Render("  no tickets checked — check tickets in the Tickets tab to build a plan"))
		return lines, nil, nil
	}

	epicName := ""
	for i, r := range rows {
		if r.epic.Name != epicName {
			epicName = r.epic.Name
			lines = append(lines, "")
			lines = append(lines, m.epicHeaderLines(r.epic)...)
			if err, ok := planErrs[epicName]; ok {
				lines = append(lines, statusErrorStyle.Render("    "+err.Error()))
			}
		}
		offsets = append(offsets, len(lines))
		rowLines := m.renderQueueTicketRow(r.epic, r.ticket, i)
		if i == m.selected {
			for li, line := range rowLines {
				rowLines[li] = ui.RenderRowHighlight(line)
			}
		}
		lines = append(lines, rowLines...)
		heights = append(heights, len(rowLines))
	}
	return lines, offsets, heights
}

// renderQueueTicketRow renders one physical line for a ticket that isn't
// currently running, and two for a live or done ticket — the same two-line
// status presentation as the Tickets tab's renderTicketRow (view.go), so the
// Queue tab shows identical per-ticket status (ticket 25).
func (m QueueModel) renderQueueTicketRow(epic tickets.Epic, t tickets.Ticket, rowIdx int) []string {
	status := epic.RenderedStatus(t)

	if status != tickets.StatusSuperseded && m.runningEpics[epic.Name] {
		if live, ok := m.live[epic.Name][t.Identifier]; ok {
			if base, suffix, ok := renderLiveTicketRow(m.icons(), m.implementSpinner, t, live, "  "); ok {
				metrics := formatMetricsLine(liveElapsedSeconds(live), live.tokens)
				return []string{base, renderRowMetricsLine(joinNonEmpty(" ", suffix, metrics), metricsLineStyle)}
			}
		}
	}

	icon, style := statusIconAndStyle(m.icons(), status)

	matched, current := m.queueSearchMatch(rowIdx)
	searchDim := m.search.HasQuery() && !matched

	title := fmt.Sprintf("%s %s", t.DisplayNumber(), t.Title)
	if t.Commitless {
		title += " (commitless)"
	}
	titleStyle := lipgloss.NewStyle()
	if matched {
		title = search.Highlight(title, m.search.Query(), current)
	} else if status == tickets.StatusDone || status == tickets.StatusSuperseded {
		titleStyle = statusDoneStyle
	} else if searchDim {
		titleStyle = ui.StyleDim
	}
	if searchDim {
		style = ui.StyleDim
	}

	line := "  " + style.Render(icon) + " " + titleStyle.Render(title)
	if suffix := blockedBySuffix(epic, t, status); suffix != "" {
		suffixStyle := blockedBySuffixStyle
		if searchDim {
			suffixStyle = ui.StyleDim
		}
		line += " " + suffixStyle.Render(suffix)
	}
	if status != tickets.StatusDone {
		return []string{line}
	}
	metrics := formatMetricsLine(t.ElapsedTime, t.ActualContextWindow)
	return []string{line, renderRowMetricsLine(metrics, style)}
}

func (m QueueModel) icons() ui.IconSet {
	return ui.Icons(m.settings.UseNerdFontIcons)
}
