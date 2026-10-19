package tickets

import (
	"context"
	"fmt"
	"sort"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/contextmenu"
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
	now          func() time.Time
	worktreeRoot string
	settings     ui.Settings
	checked      map[string]bool
	checkOrder   map[string]uint64
	// live is epicName -> ticket identifier -> running state, derived from the
	// delivered state (syncServerRunState), so the Queue tab's rows render the
	// running spinner+phase presentation (renderLiveTicketRow).
	live map[string]map[string]liveTicketState
	// summaries and herdrDown come from the delivered state: the server's
	// summary of each queued epic by epic name, and whether the server can
	// launch agents.
	summaries map[string]server.EpicSummary
	herdrDown bool
	// finished is the snapshot's Recently finished list. finishedGen numbers the
	// refresh tick chain: each page activation starts a new chain and the older
	// chain's pending tick is ignored.
	finished    []server.FinishedEpic
	finishedGen int
	// serverBudget is the last snapshot's budget: the header's spend and the
	// root rows' cost in server mode.
	serverBudget     server.BudgetStatus
	implementSpinner spinner.Model

	width, height int
	ready         bool
	loaded        bool
	epics         []tickets.Epic
	candidates    map[string]bool

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
	// Cuts the CPU cost of rebuilding the full tree from scratch every
	// render even when nothing changed.
	entriesCache *queueEntriesCache

	// ticketMenu and ticketMenuPending mirror the Tickets tab's own (see
	// queue_ticket_menu.go).
	ticketMenu        contextmenu.Menu[ticketTarget]
	ticketMenuPending string
	watch             watchModal
	runningEpics      map[string]bool
	paused            bool

	// search backs "/"-triggered filtering over buildQueueEntries, mirroring
	// the Tickets tab's own m.search (see ui/tickets/search.go).
	search search.Model

	// confirm backs the "C"/"c" clear keymaps and the "x" delete keymap
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

	// serverAPI/serverStart/link back the server-down banner (see
	// queue_server_down.go). link is only ever set by WithServerState; a tab
	// with no client is delivered ServerLinkDown, so it shows the banner too.
	serverAPI   ServerAPI
	serverStart func(context.Context) error
	link        ServerLink
	// olderServer: see Model.olderServer.
	olderServer bool
	// projectFilter narrows the server-mode rows to one project; "" shows all.
	projectFilter string
	// focusEpic is the address of an epic to select when its rows arrive.
	focusEpic string
}

func NewQueueModel(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) QueueModel {
	sp := spinner.New()
	sp.Spinner = ui.ProgressSpinnerFor(settings.UseNerdFontIcons)
	km := newQueueKeysManager()
	queueTree := tree.NewModel[queueNode]()
	queueTree.SetIsSelectable(func(n queueNode) bool {
		switch n.kind {
		case nodeEpicSeparator, nodeEpicBar, nodeEpicStatusLine, nodeEpicSpacer, nodeEpicError:
			return false
		default:
			return true
		}
	})
	return QueueModel{
		now:              time.Now,
		worktreeRoot:     worktreeRoot,
		settings:         settings,
		checked:          map[string]bool{},
		checkOrder:       map[string]uint64{},
		live:             map[string]map[string]liveTicketState{},
		implementSpinner: sp,
		runningEpics:     map[string]bool{},
		confirm:          confirm.New(),
		search:           search.NewModel(),
		keys:             km,
		queueTree:        queueTree,
		entriesCache:     &queueEntriesCache{},
		help:             help.NewModel(help.BuildSections(km, *queueTree.Keys(), extraKeys), settings.About),
		previewFocus:     newPreviewFocus(),
	}
}

func (m QueueModel) Init() tea.Cmd {
	return nil // the app shell delivers the state (WithServerState)
}

// finishedRefreshInterval is how often the Recently finished times re-render.
const finishedRefreshInterval = time.Minute

// finishedTickMsg re-renders the relative times of the Recently finished list.
type finishedTickMsg struct{ gen int }

func finishedTick(gen int) tea.Cmd {
	return tea.Tick(finishedRefreshInterval, func(time.Time) tea.Msg { return finishedTickMsg{gen: gen} })
}

// restartFinishedTick starts a new refresh chain, orphaning the old one, or
// returns nil when there is no finished entry to refresh.
func (m *QueueModel) restartFinishedTick() tea.Cmd {
	m.finishedGen++
	if len(m.finished) == 0 {
		return nil
	}
	return finishedTick(m.finishedGen)
}

// queueSpinnerRestartMsg restarts the running spinner on page activation.
type queueSpinnerRestartMsg struct{}

// Update delegates to updateInner then re-syncs the preview viewport,
// mirroring the Tickets tab's own Update/syncPreviewViewport split (see
// model.go) so every message that can move the selection, resize the
// panels, or reload data doesn't need to remember to do it itself.
func (m QueueModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.updateInner(msg)
	nm := next.(QueueModel)
	if nm.ticketMenuPending != nm.selectedQueuePath() {
		nm.ticketMenuPending = ""
	}
	nm.refreshQueueHover(msg)
	nm.syncQueuePreviewViewport()
	return nm, cmd
}

func (m QueueModel) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	if nav.IsOpenHelp(msg) {
		if m.ModalOpen() {
			return m, nil
		}
		var cmd tea.Cmd
		m, cmd = nav.EndSearch(m, m.search.InputFocused())
		m.keys.Reset()
		m.help.Open(m.width, m.height)
		return m, cmd
	}
	if next, cmd, ok := m.updateServerDown(msg); ok {
		return next, cmd
	}
	if next, cmd, ok := m.updateServerKeys(msg); ok {
		return next, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.help, _ = m.help.Update(msg)
		if m.loaded {
			// The epic header layout depends on the width (see buildQueueEntries).
			m.queueTree.SetEntries(m.buildQueueEntriesCached())
		}
		m.queueTree.SetVisibleHeight(m.queueViewportHeight() - queueHeaderReservedLines)
		return m, nil
	case queueSpinnerRestartMsg:
		// The tick chain dies while the tab is hidden, so a claim that arrived
		// meanwhile would otherwise show no spinner.
		// The finished-time refresh chain dies the same way, so activation
		// starts a new one and orphans any tick still pending from the old one.
		tick := m.restartFinishedTick()
		if len(m.runningEpics) == 0 {
			return m, tick
		}
		return m, tea.Batch(m.implementSpinner.Tick, tick)
	case spinner.TickMsg:
		return m.handleQueueSpinnerTick(msg)
	case finishedTickMsg:
		if msg.gen != m.finishedGen {
			return m, nil
		}
		return m, finishedTick(m.finishedGen)
	case tea.MouseWheelMsg:
		if next, cmd, handled := m.help.Forward(msg); handled {
			m.help = next
			return m, cmd
		}
		return m.handleQueueMouseWheel(msg)
	case tea.MouseMotionMsg:
		return m.handleQueueMouseMotion(msg)
	case editFileFinishedMsg:
		return m.handleEditFileFinished(msg)
	case answerEditorFinishedMsg:
		var cmd tea.Cmd
		m.confirm, cmd = handleAnswerEditorFinished(m.confirm, msg)
		return m, cmd
	case watchLoadedMsg:
		m.watch = m.watch.Open(msg, m.width, m.height)
		return m, m.watch.tick()
	case watchStepMsg:
		var cmd tea.Cmd
		m.watch, cmd = m.watch.Step(msg)
		return m, cmd
	case ticketMenuReadyMsg:
		return m.handleTicketMenuReady(msg), nil
	case ticketMenuPickMsg:
		return m.handleTicketMenuPick(msg)
	case epicMenuPickMsg:
		return m.handleQueueEpicMenuPick(msg)
	case tea.KeyPressMsg:
		if m.help.IsOpen {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}
		if m.watch.open {
			var cmd tea.Cmd
			m.watch, cmd = m.watch.Update(msg)
			return m, cmd
		}
		if m.ticketMenu.IsOpen {
			var cmd tea.Cmd
			m.ticketMenu, cmd = m.ticketMenu.Update(msg)
			return m, cmd
		}
		if m.confirm.IsOpen {
			return m.handleQueueConfirmUpdate(msg)
		}
		if msg.String() == "esc" {
			m.ticketMenuPending = ""
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
		if m.ticketMenu.IsOpen {
			var cmd tea.Cmd
			m.ticketMenu, cmd = m.ticketMenu.UpdateMouse(msg, m.menuAnchor(), m.width, m.height)
			return m, cmd
		}
		if m.watch.open {
			return m, nil
		}
		if m.confirm.IsOpen {
			return m.handleQueueConfirmMouseUpdate(msg)
		}
		return m.handleQueueMouseClick(msg)
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
	sidebarW, _ := m.panelWidths(m.width)
	sidebarH, _ := m.panelHeights(m.width, m.contentHeight())
	return ui.Rect{X: 0, Y: 0, W: sidebarW, H: sidebarH}
}

// queuePreviewRect wraps the shared previewRect in a ui.Rect for
// HoverHitTest.
func (m QueueModel) queuePreviewRect() ui.Rect {
	x, y, w, h := m.previewRect()
	return ui.Rect{X: x, Y: y, W: w, H: h}
}

// contentHeight mirrors Model.contentHeight for the Queue tab's own layout.
func (m QueueModel) contentHeight() int {
	return max(m.height, 1)
}

// previewRect returns the preview panel's absolute on-screen bounds,
// mirroring Model.previewRect for the Queue tab's own layout.
func (m QueueModel) previewRect() (x, y, w, h int) {
	return m.previewFocus.previewRect(m.width, m.contentHeight())
}

// handleQueueMouseClick selects the row under the click without triggering
// any secondary action (no confirm, no checkbox toggle). A click inside the
// preview panel's bounds instead hands focus to it (mirroring Model's
// handleSidebarMouseClick), without changing the queue selection. A right
// click on a ticket row or epic header selects it and opens its Context menu
// as "m" does; anywhere else it does nothing.
func (m QueueModel) handleQueueMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft && mouse.Button != tea.MouseRight {
		return m, nil
	}
	if m.previewFocus.inPreview(mouse, m.width, m.contentHeight()) {
		if mouse.Button == tea.MouseLeft {
			m.previewFocus.focusPreviewPane()
		}
		return m, nil
	}
	bodyLine := mouse.Y - 1 - queueHeaderReservedLines
	if mouse.Button == tea.MouseRight {
		idx, ok := m.queueTree.EntryAtBodyLine(bodyLine)
		if !ok {
			return m, nil
		}
		if kind := m.queueTree.Entries()[idx].Value.kind; kind != nodeQueueTicket && kind != nodeEpicName {
			return m, nil
		}
		m.queueTree.SetSelectedIndex(idx)
		return m.handleQueueTicketMenuKey()
	}
	if idx, ok := m.queueTree.EntryAtBodyLine(bodyLine); ok {
		entry := m.queueTree.Entries()[idx]
		// Only ticket rows draw a triangle; the label's leading cell is the
		// indent renderQueueTicketRow puts before it.
		if entry.Value.kind == nodeQueueTicket && onTreeIcon(mouse.X, entry, 1, m.icons()) && m.queueTree.ToggleEntry(idx) {
			m.clampSelected()
			if m.search.HasQuery() {
				m.recomputeQueueSearchMatches()
			}
			return m, nil
		}
		m.queueTree.SetSelectedIndex(idx)
	}
	return m, nil
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

// bindingQueueToggleHideDone is the Queue tab's "tc" chord (ticket 09),
// dispatched through keys.Manager so a key typed right after an unconsumed
// "t" falls through to its own normal action instead of being swallowed
// (ticket 16).
const bindingQueueToggleHideDone keys.BindingID = "toggle-hide-done"

// bindingQueueCycleProject is "tp": all projects, then each project in turn.
const bindingQueueCycleProject keys.BindingID = "cycle-project"

const bindingQueueTogglePreview keys.BindingID = "toggle-preview"

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
// bindingQueueTicketMenu were previously handled by a raw
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
	bindingQueueTicketMenu       keys.BindingID = "ticket-menu"
	bindingQueueStartServer      keys.BindingID = "start-server"
	bindingQueueApprove          keys.BindingID = "approve-proposal"
)

func newQueueKeysManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingQueueHelp, Seq: []string{"?"}, Categories: []string{"Other"}, Title: "help"},
		{ID: bindingQueueToggleHideDone, Seq: []string{"t", "c"}, Categories: []string{"Navigation"}, Title: "hide completed"},
		{ID: bindingQueueCycleProject, Seq: []string{"t", "p"}, Categories: []string{"Navigation"}, Title: "filter by project"},
		{ID: bindingQueueTogglePreview, Seq: []string{"t", "v"}, Categories: []string{"Navigation"}, Title: "toggle preview"},
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
		{ID: bindingQueueTicketMenu, Seq: []string{"m"}, Categories: []string{"Other"}, Title: "ticket actions"},
		{ID: bindingQueueStartServer, Seq: []string{"s"}, Categories: []string{"Other"}, Title: "start server (when down)"},
		{ID: bindingQueueApprove, Seq: []string{"A"}, Categories: []string{"Other"}, Title: "approve recovery proposal"},
	})
}

func (m QueueModel) handleQueueKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		case bindingQueueCycleProject:
			if m.serverAPI == nil {
				return m, nil
			}
			m.projectFilter = nextProject(m.epics, m.projectFilter)
			m.clampSelected()
			label := "all projects"
			if m.projectFilter != "" {
				label = "project: " + m.projectFilter
			}
			return m, notify.Info(label)
		case bindingQueueTogglePreview:
			m.togglePreview()
			m.queueTree.SetVisibleHeight(m.queueViewportHeight() - queueHeaderReservedLines)
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
			// The shell owns the connection and the snapshot; it hands the result
			// back as state.
			if m.serverDown() {
				return m, cmdProbeRequested
			}
			return m, func() tea.Msg { return ResnapshotRequestedMsg{} }
		case bindingQueuePauseResume:
			if m.serverAPI != nil {
				return m.handleServerPauseKey()
			}
		case bindingQueueClearChecked:
			if paths := m.checkedPaths(); len(paths) > 0 && m.serverAPI != nil {
				return m.handleServerClearKey(fmt.Sprintf("Dequeue all %d ticket(s)?", len(paths)), paths)
			}
		case bindingQueueClearDoneChecked:
			if paths := m.doneCheckedPaths(); len(paths) > 0 && m.serverAPI != nil {
				return m.handleServerClearKey(fmt.Sprintf("Dequeue %d completed ticket(s)?", len(paths)), paths)
			}
		case bindingQueueDelete:
			if m.serverAPI == nil {
				return m, notify.Warning(serverNotRunning)
			}
			return m.handleServerDeleteKey()
		case bindingQueueTicketMenu:
			return m.handleQueueTicketMenuKey()
		case bindingQueueApprove:
			return m.handleServerApproveKey()
		case bindingQueueStartServer:
			return m.openServerStartConfirm(), nil
		}
		return m, nil
	}
	if s := msg.String(); s == "q" || s == "esc" {
		return m, nav.Back()
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
		m.focusPreviewPane()
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
		m.focusPreviewPane()
	}
	return m, cmd
}

type checkedEpicPlan struct {
	epic      tickets.Epic
	ticketIDs []string
	ordinal   uint64
	ordered   bool
}

func (m QueueModel) checkedEpicPlans() []checkedEpicPlan {
	return checkedEpicPlansFor(m.epics, m.checked, m.checkOrder)
}

func checkedEpicPlansFor(epics []tickets.Epic, checked map[string]bool, checkOrder map[string]uint64) []checkedEpicPlan {
	plans := make([]checkedEpicPlan, 0, len(epics))
	for _, epic := range epics {
		var ticketIDs []string
		var ordinal uint64
		ordered := false
		for _, idx := range sortedTicketIndexes(epic) {
			ticket := epic.Tickets[idx]
			if !checked[ticket.Path] {
				continue
			}
			ticketIDs = append(ticketIDs, ticket.Identifier)
			if checkedAt, ok := checkOrder[ticket.Path]; ok && (!ordered || checkedAt < ordinal) {
				ordinal, ordered = checkedAt, true
			}
		}
		if len(ticketIDs) > 0 {
			plans = append(plans, checkedEpicPlan{epic: epic, ticketIDs: ticketIDs, ordinal: ordinal, ordered: ordered})
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
	m.selectFocusEpic()
}

// WithFocusEpic makes the tab select the header of the epic at address as
// soon as its rows arrive.
func (m QueueModel) WithFocusEpic(address string) QueueModel {
	m.focusEpic = address
	m.selectFocusEpic()
	return m
}

// FocusEpic is the epic the tab still waits to select; "" once it did.
func (m QueueModel) FocusEpic() string { return m.focusEpic }

// selectFocusEpic selects the focus epic's header once the rows hold it; until
// then the request waits for the next delivery.
func (m *QueueModel) selectFocusEpic() {
	if m.focusEpic == "" {
		return
	}
	for i, e := range m.queueTree.Entries() {
		if e.Value.kind == nodeEpicName && (e.Value.epic.Path == m.focusEpic || e.Value.epic.Name == m.focusEpic) {
			m.queueTree.SetSelectedIndex(i)
			m.focusEpic = ""
			return
		}
	}
}

// queueViewportHeight is the queue panel's visible body line count, matching
// ui.RenderPanel's own bodyH math (PaddingY: 0, minus the header row) — see
// View()'s split sizing — so the windowing done here lines up with what
// RenderPanel actually paints. Splits its height the same way View() does
// (splitPanelHeight), since a stacked (narrow-terminal) layout shares the
// available height with the preview pane below it.
func (m QueueModel) queueViewportHeight() int {
	sidebarH, _ := m.panelHeights(m.width, m.contentHeight())
	return max(sidebarH-1, 0)
}
