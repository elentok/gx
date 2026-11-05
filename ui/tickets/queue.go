package tickets

import (
	"fmt"
	"sort"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/ui/tree"
)

// QueueModel renders a checked selection as dependency-aware epic waves.
type QueueModel struct {
	now          func() time.Time
	worktreeRoot string
	settings     ui.Settings
	editor       rowactions.Editor
	// server is the delivered state and the link it came over (queue_server_live.go).
	server           queueServer
	implementSpinner spinner.Model

	width, height int
	ready         bool
	loaded        bool

	// rows is the tree builder and row renderer with the state they own (see
	// queue_rows.go). The header is stateless: header() builds it per call.
	rows queueRows

	// menus is the Tickets tab's row menu plus the Queue group.
	menus rowactions.Menus
	watch rowactions.WatchModal

	// search backs "/"-triggered filtering over buildQueueEntries, mirroring
	// the Tickets tab's own m.search (see ui/tickets/search.go).
	search search.Model

	// confirm backs the "C"/"c" clear keymaps and the "x" delete keymap
	// (handleQueueKey) — the Queue tab is read-only for selection (ticket 08),
	// so this is the only modal this tab opens outside the agent-picker menu.
	confirm confirm.Model

	// harnessPicker is the "H" Default harness modal.
	harnessPicker harnessPicker

	// keys dispatches the "tc" chord above through ui/keys.Manager so a key
	// typed right after an unconsumed "t" falls through to its own normal
	// action instead of being swallowed (ticket 16).
	keys keys.Manager
	help help.Model

	// preview backs the preview panel's scroll/search machinery, shared
	// with the Tickets tab (see preview_focus.go). Its focus field is set by
	// "l"/"right"/"enter" and "h"/"left"/"esc" (see handleQueueKey and
	// handleQueuePreviewKey in queue_preview.go).
	preview preview.Pane

	// srv holds the sub-clients the server-mode keys call (server_queue_keys.go).
	srv      serverKeys
	sessions tabapi.SessionsAPI
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
		now:          time.Now,
		worktreeRoot: worktreeRoot,
		settings:     settings,
		editor:       rowactions.Editor{WorktreeRoot: worktreeRoot, Settings: settings},
		rows:         newQueueRows(queueTree),
		server: queueServer{
			live:         map[string]map[string]rowfmt.LiveTicketState{},
			runningEpics: map[string]bool{},
		},
		implementSpinner: sp,
		confirm:          confirm.New(),
		menus:            rowactions.Menus{OnQueue: true},
		search:           search.NewModel(),
		keys:             km,
		help:             help.NewModel(help.BuildSections(km, *queueTree.Keys(), extraKeys), settings.About),
		preview:          preview.NewPane(),
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

// queueSpinnerRestartMsg restarts the running spinner on page activation.
type queueSpinnerRestartMsg struct{}

// Update delegates to updateInner then re-syncs the preview viewport,
// mirroring the Tickets tab's own Update/syncPreviewViewport split (see
// model.go) so every message that can move the selection, resize the
// panels, or reload data doesn't need to remember to do it itself.
func (m QueueModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.updateInner(msg)
	nm := next.(QueueModel)
	if nm.menus.Pending != nm.selectedQueuePath() {
		nm.menus.Pending = ""
	}
	// An open modal clears the hover, and a wheel scroll moves other rows under
	// the still cursor.
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		nm.rows.queueTree.SetHoveredIndex(nm.queueHoverTarget(tea.MouseMotionMsg(wheel)))
	} else if nm.ModalOpen() {
		nm.rows.queueTree.SetHoveredIndex(0, false)
	}
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
	if cmd, ok := m.server.updateDown(msg); ok {
		return m, cmd
	}
	if cmd, open, ok := m.srv.update(msg, m.server.paused); ok {
		if open != nil {
			m.confirm = m.confirm.Open(*open)
		}
		return m, cmd
	}
	if picker, cmd, ok := m.harnessPicker.update(msg, m.rows.projectFilter); ok {
		m.harnessPicker = picker
		return m, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.help, _ = m.help.Update(msg)
		if m.loaded {
			// The epic header layout depends on the width (see buildQueueEntries).
			m.rows.queueTree.SetEntries(m.rows.buildQueueEntriesCached(m.rowsEnv()))
		}
		m.rows.queueTree.SetVisibleHeight(m.layout().viewportHeight() - queueHeaderReservedLines)
		return m, nil
	case queueSpinnerRestartMsg:
		// The tick chain dies while the tab is hidden, so a claim that arrived
		// meanwhile would otherwise show no spinner.
		// The finished-time refresh chain dies the same way, so activation
		// starts a new one and orphans any tick still pending from the old one.
		var tick tea.Cmd
		m.server, tick = m.server.restartFinishedTick()
		if len(m.server.runningEpics) == 0 {
			return m, tick
		}
		return m, tea.Batch(m.implementSpinner.Tick, tick)
	case spinner.TickMsg:
		if len(m.server.runningEpics) == 0 {
			return m, nil
		}
		var cmd tea.Cmd
		m.implementSpinner, cmd = m.implementSpinner.Update(msg)
		return m, cmd
	case finishedTickMsg:
		if msg.gen != m.server.finishedGen {
			return m, nil
		}
		return m, finishedTick(m.server.finishedGen)
	case tea.MouseWheelMsg:
		if next, cmd, handled := m.help.Forward(msg); handled {
			m.help = next
			return m, cmd
		}
		return m.handleQueueMouseWheel(msg)
	case tea.MouseMotionMsg:
		m.rows.queueTree.SetHoveredIndex(m.queueHoverTarget(msg))
		return m, nil
	case rowactions.EditFileFinishedMsg:
		return m.handleEditFileFinished(msg)
	case rowactions.WatchLoadedMsg:
		m.watch = m.watch.Open(msg, m.width, m.height)
		return m, m.watch.Tick()
	case rowactions.WatchStepMsg:
		var cmd tea.Cmd
		m.watch, cmd = m.watch.Step(msg)
		return m, cmd
	case rowactions.TicketReadyMsg:
		cmd := m.menus.HandleReady(msg, queueMenuTab{m})
		return m, cmd
	case rowactions.TicketPickMsg:
		res := m.menus.HandleTicketPick(msg, queueMenuTab{m})
		if res.Approve {
			return m.handleServerApproveKey()
		}
		return m, res.Cmd
	case rowactions.EpicPickMsg:
		return m, m.menus.HandleEpicPick(msg, queueMenuTab{m})
	case tea.KeyPressMsg:
		if m.help.IsOpen {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}
		if m.watch.IsOpen() {
			var cmd tea.Cmd
			m.watch, cmd = m.watch.Update(msg)
			return m, cmd
		}
		if m.menus.Menu.IsOpen {
			var cmd tea.Cmd
			m.menus.Menu, cmd = m.menus.Menu.Update(msg)
			return m, cmd
		}
		if m.confirm.IsOpen {
			var cmd tea.Cmd
			m.confirm, cmd, _ = m.confirm.Update(msg)
			return m, cmd
		}
		if m.harnessPicker.open {
			var cmd tea.Cmd
			m.harnessPicker, cmd = m.harnessPicker.updateKey(msg, m.srv.admin)
			return m, cmd
		}
		if msg.String() == "esc" {
			m.menus.Pending = ""
		}
		if m.preview.Focus == preview.FocusPreview {
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
		if m.menus.Menu.IsOpen {
			var cmd tea.Cmd
			m.menus.Menu, cmd = m.menus.Menu.UpdateMouse(msg, m.menuAnchor(), m.width, m.height)
			return m, cmd
		}
		if m.watch.IsOpen() {
			return m, nil
		}
		if m.confirm.IsOpen {
			var cmd tea.Cmd
			m.confirm, cmd, _ = m.confirm.UpdateMouse(msg, m.width, m.width, m.height)
			return m, cmd
		}
		if m.harnessPicker.open {
			return m, nil
		}
		return m.handleQueueMouseClick(msg)
	}
	return m, nil
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
	idx, ok := ui.HoverHitTest(mouse.X, mouse.Y, m.layout().treeRect(), m.layout().previewRect())
	if !ok {
		return m, nil
	}
	if idx == 1 {
		var cmd tea.Cmd
		m.preview.VP, cmd = m.preview.VP.Update(msg)
		return m, cmd
	}
	m.rows.queueTree.ScrollViewport(dir * ui.WheelScrollLines)
	return m, nil
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
	if m.preview.InPreview(mouse, m.width, m.layout().contentHeight()) {
		if mouse.Button == tea.MouseLeft {
			m.preview.FocusPreviewPane()
		}
		return m, nil
	}
	bodyLine := mouse.Y - 1 - queueHeaderReservedLines
	if mouse.Button == tea.MouseRight {
		idx, ok := m.rows.queueTree.EntryAtBodyLine(bodyLine)
		if !ok {
			return m, nil
		}
		if kind := m.rows.queueTree.Entries()[idx].Value.kind; kind != nodeQueueTicket && kind != nodeEpicName {
			return m, nil
		}
		m.rows.queueTree.SetSelectedIndex(idx)
		return m.handleQueueTicketMenuKey()
	}
	if idx, ok := m.rows.queueTree.EntryAtBodyLine(bodyLine); ok {
		entry := m.rows.queueTree.Entries()[idx]
		// Only ticket rows draw a triangle; the label's leading cell is the
		// indent renderQueueTicketRow puts before it.
		if entry.Value.kind == nodeQueueTicket && rowfmt.OnTreeIcon(mouse.X, entry, 1, m.icons()) && m.rows.queueTree.ToggleEntry(idx) {
			m.clampSelected()
			if m.search.HasQuery() {
				m.recomputeQueueSearchMatches()
			}
			return m, nil
		}
		m.rows.queueTree.SetSelectedIndex(idx)
	}
	return m, nil
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
	bindingQueueDefaultHarness   keys.BindingID = "default-harness"
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
		{ID: bindingQueueDefaultHarness, Seq: []string{"H"}, Categories: []string{"Other"}, Title: "Default harness..."},
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
			m.rows.hideComplete = !m.rows.hideComplete
			m.clampSelected()
		case bindingQueueCycleProject:
			if m.server.serverAPI == nil {
				return m, nil
			}
			m.rows.projectFilter = nextProject(m.rows.epics, m.rows.projectFilter)
			m.clampSelected()
			label := "all projects"
			if m.rows.projectFilter != "" {
				label = "project: " + m.rows.projectFilter
			}
			return m, notify.Info(label)
		case bindingQueueTogglePreview:
			m.preview.Toggle()
			m.rows.queueTree.SetVisibleHeight(m.layout().viewportHeight() - queueHeaderReservedLines)
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
			m.rows.selectFirstRow()
		case bindingQueueYankSummary:
			return m, m.yankQueueTicketSummary()
		case bindingQueueYankFilePath:
			return m, m.yankQueueTicketFilePath()
		case bindingQueueSelectLast:
			m.rows.selectLastRow()
		case bindingQueuePreviewBottom:
			m.preview.VP.GotoBottom()
		case bindingQueueReload:
			// The shell owns the connection and the snapshot; it hands the result
			// back as state.
			if m.server.down() {
				return m, cmdProbeRequested
			}
			return m, func() tea.Msg { return tabapi.ResnapshotRequestedMsg{} }
		case bindingQueuePauseResume:
			if m.server.serverAPI != nil {
				return m, m.srv.cmdPause()
			}
		case bindingQueueClearChecked:
			if paths := m.checkedPaths(); len(paths) > 0 && m.server.serverAPI != nil {
				m.confirm = m.confirm.Open(m.srv.clearConfirm(fmt.Sprintf("Dequeue all %d ticket(s)?", len(paths)), paths))
			}
		case bindingQueueClearDoneChecked:
			if paths := m.doneCheckedPaths(); len(paths) > 0 && m.server.serverAPI != nil {
				m.confirm = m.confirm.Open(m.srv.clearConfirm(fmt.Sprintf("Dequeue %d completed ticket(s)?", len(paths)), paths))
			}
		case bindingQueueDelete:
			if m.server.serverAPI == nil {
				return m, notify.Warning(rowactions.ServerNotRunning)
			}
			row, ok := m.selectedQueueRow()
			if !ok {
				return m, nil
			}
			if !m.rows.checked[row.ticket.Path] {
				return m, notify.Info(row.ticket.DisplayNumber() + " is not a queue item")
			}
			m.confirm = m.confirm.Open(m.srv.deleteConfirm(row.ticket))
		case bindingQueueTicketMenu:
			return m.handleQueueTicketMenuKey()
		case bindingQueueApprove:
			return m.handleServerApproveKey()
		case bindingQueueDefaultHarness:
			return m, m.harnessPicker.readCmd(m.server, m.srv.admin)
		case bindingQueueStartServer:
			if opts, ok := m.server.startConfirm(); ok {
				m.confirm = m.confirm.Open(opts)
			}
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
	next, cmd, result := m.rows.queueTree.Update(msg)
	if result.ExpandNoop {
		m.preview.FocusPreviewPane()
		return m, cmd
	}
	m.rows.queueTree = next
	if result.RebuildRequested {
		m.clampSelected()
		if m.search.HasQuery() {
			m.recomputeQueueSearchMatches()
		}
	}
	if result.OpenSelected {
		m.preview.FocusPreviewPane()
	}
	return m, cmd
}

type checkedEpicPlan struct {
	epic      tickets.Epic
	ticketIDs []string
	ordinal   uint64
	ordered   bool
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

// header builds the stateless header from the tab's current inputs.
func (m QueueModel) header() queueHeader {
	h := queueHeader{
		icons:        m.icons(),
		summaries:    m.server.summaries,
		runningEpics: m.server.runningEpics,
		live:         m.server.live,
		paused:       m.server.paused,
		serverDown:   m.server.down(),
		hasServer:    m.server.serverAPI != nil,
		herdrDown:    m.server.herdrDown,
		herdrText:    m.server.herdrText,
		budget:       m.server.serverBudget,
		finished:     m.server.finished,
		spinner:      m.implementSpinner,
		now:          m.now,
		checkedCount: len(m.rows.checked),
	}
	if m.server.serverAPI == nil {
		h.checkedPlans = len(checkedEpicPlansFor(m.rows.epics, m.rows.checked, m.rows.checkOrder))
	}
	return h
}

// rowsEnv gathers what queueRows reads from the rest of the tab.
func (m QueueModel) rowsEnv() queueRowsEnv {
	return queueRowsEnv{
		header:        m.header(),
		maxParallel:   m.settings.MaxConcurrentTicketsPerEpic(),
		labelWidth:    m.layout().labelWidth(),
		harnessLabels: m.server.harnessLabels,
		waitingLabels: m.server.waitingLabels,
		codexRows:     m.server.codexRows,
		hasQuery:      m.search.HasQuery(),
		sidebarActive: m.preview.Focus == preview.FocusSidebar,
		suggested: func(epic tickets.Epic, ticket tickets.Ticket) bool {
			return m.menus.HasSuggestedActions(m.menuEnv(), epic, ticket, false)
		},
	}
}

// clampSelected rebuilds the queue tree's entries from the current
// epics/hideComplete/collapse state — SetEntries re-clamps selection to the
// new entry count.
func (m *QueueModel) clampSelected() {
	m.rows.queueTree.SetEntries(m.rows.buildQueueEntries(m.rowsEnv()))
	m.rows.queueTree.SkipUnselectable(1)
	m.focusEpic = m.rows.selectFocusEpic(m.focusEpic)
}

// WithFocusEpic makes the tab select the header of the epic at address as
// soon as its rows arrive.
func (m QueueModel) WithFocusEpic(address string) QueueModel {
	m.focusEpic = m.rows.selectFocusEpic(address)
	return m
}
