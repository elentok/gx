// Package tickets implements the `gx tickets` tab: a sidebar+preview pairing
// (the worktrees archetype per ADR 0009) over the repo's local `.scratch/`
// issue tracker.
package tickets

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/sidebar"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/ui/tree"
)

// Model is the top-level tickets tab model: an epic/ticket sidebar paired
// with a focusable preview panel that mirrors the sidebar's selection (see
// GLOSSARY.md's panel vocabulary) — "l"/"enter" on a ticket row hands focus to
// it for scrolling/searching its body; "h"/"left"/"esc" hands focus back.
type Model struct {
	worktreeRoot string
	settings     ui.Settings
	editor       rowactions.Editor
	keys         keys.Manager // this tab's own navigation/collapse bindings
	help         help.Model

	width  int
	height int
	ready  bool // true once the first WindowSizeMsg has been received

	loaded bool
	// sidebar owns the epic rows and the sidebar tree's state (sidebar.go).
	sidebar sidebar.Model
	search  search.Model

	// preview backs the preview panel's own focus/scroll/search state
	// (see preview_focus.go and model_preview_focus.go).
	preview preview.Pane

	confirm confirm.Model

	// menus is the "m"-triggered Context menu on a ticket or epic row.
	menus rowactions.Menus

	// watch backs the Watch agent modal (watch_agent.go).
	watch rowactions.WatchModal

	// server owns the server link, snapshot and project scope (server_mode.go).
	server   serverBinding
	admin    tabapi.AdminAPI
	sessions tabapi.SessionsAPI
	tickets  tabapi.TicketsAPI
	queue    tabapi.QueueAPI
	reads    tabapi.ReadsAPI
}

// NewModel creates a new tickets tab model scoped to worktreeRoot's own
// `.scratch/`. extraKeys (the app-wide global bindings) feeds the "?" help
// modal alongside the tab's own bindings, mirroring ui/prs's NewModelWithScope.
func NewModel(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) Model {
	km := newTicketsManager()
	sb := sidebar.New(tabapi.ScratchDir(settings.TicketStore, worktreeRoot))
	return Model{
		worktreeRoot: worktreeRoot,
		settings:     settings,
		editor:       rowactions.Editor{WorktreeRoot: worktreeRoot, Settings: settings},
		keys:         km,
		sidebar:      sb,
		help:         help.NewModel(help.BuildSections(km, *sb.Tree.Keys(), extraKeys), settings.About),
		search:       search.NewModel(),
		preview:      preview.NewPane(),
		confirm:      confirm.New(),
	}
}

func (m Model) KeyManager() keys.Manager { return m.keys }

// InputFocused reports whether either search box is mid-input, so the app
// shell's digit-based tab-jump mnemonics (see ui/app's inputFocuser
// duck-type) stay routed to the search query instead of switching tabs.
func (m Model) InputFocused() bool {
	if m.help.InputFocused() || m.menus.Menu.IsOpen {
		return true
	}
	_, ok := m.activeInputSearch()
	return ok
}

// ModalOpen reports whether one of the tab's launch dialogs is open, so the app
// shell (see ui/app's modalOpener duck-type) blocks tab-switch keys and
// routes them here instead while it's up.
func (m Model) ModalOpen() bool {
	return m.help.IsOpen || m.menus.Menu.IsOpen || m.confirm.IsOpen || m.watch.IsOpen()
}

func (m Model) Init() tea.Cmd {
	return m.OnPageActivated() // else the app shell's stream delivers the state
}

// Update delegates to updateInner then re-syncs the preview viewport
// (content/size/scroll-reset-on-selection-change) against whatever the
// message just changed, so every call site that can move the selection,
// resize the panels, or reload data doesn't need to remember to do it itself.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.updateInner(msg)
	nm := next.(Model)
	if nm.menus.Pending != nm.selectedTicketPath() {
		nm.menus.Pending = ""
	}
	refreshHover(&nm, msg)
	nm.syncPreviewViewport()
	return nm, cmd
}

func (m Model) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	if nav.IsOpenHelp(msg) {
		if m.ModalOpen() {
			return m, nil
		}
		_, searching := m.activeInputSearch()
		var cmd tea.Cmd
		m, cmd = nav.EndSearch(m, searching)
		m.keys.Reset()
		m.help.Open(m.width, m.height)
		return m, cmd
	}
	if cmd, ok := updateServer(msg); ok {
		return m, cmd
	}
	switch msg := msg.(type) {
	case tabapi.StoreChangedMsg:
		return m, m.OnPageActivated()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.help, _ = m.help.Update(msg)
		m.sidebar.Tree.SetVisibleHeight(m.layout().sidebarViewportHeight())
		return m, nil

	case epicsLoadedMsg:
		if !m.server.readsDisk() {
			// A disk read that outlived down mode must not overwrite stream rows.
			return m, nil
		}
		firstLoad := !m.loaded
		m.loaded = true
		m.sidebar.Epics = msg.epics
		m.sidebar.ArchivedEpicCount = msg.archivedEpicCount
		m.sidebar.ArchivedLazy.SetCount(msg.archivedEpicCount)
		if m.search.HasQuery() {
			m.recomputeSearchMatches()
		}
		m.clampSelected()
		if firstLoad {
			m.sidebar.SelectFirstEpic()
		}
		if msg.err != nil {
			return m, notify.Error("load .scratch/: " + msg.err.Error())
		}
		return m, nil

	case tree.LazyResultMsg[tickets.Epic]:
		if m.sidebar.ArchivedLazy.Deliver(msg) {
			m.sidebar.ArchivedEpics = m.sidebar.ArchivedLazy.Rows()
			m.clampSelected()
		}
		return m, nil

	case rowactions.EditFileFinishedMsg:
		return m, m.editor.Finished(msg)

	case rowactions.WatchLoadedMsg:
		m.watch = m.watch.Open(msg, m.width, m.height)
		return m, m.watch.Tick()
	case rowactions.WatchStepMsg:
		var cmd tea.Cmd
		m.watch, cmd = m.watch.Step(msg)
		return m, cmd

	case rowactions.TicketReadyMsg:
		cmd := m.menus.HandleReady(msg, modelMenuTab{m})
		return m, cmd

	case rowactions.TicketPickMsg:
		return m.handleTicketMenuPick(msg)

	case rowactions.EpicPickMsg:
		return m, m.menus.HandleEpicPick(msg, modelMenuTab{m})

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
		if msg.String() == "esc" {
			m.menus.Pending = ""
		}
		return m.handleKey(msg)

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
		if _, ok := m.activeInputSearch(); ok {
			return m, nil
		}
		return m.handleSidebarMouseClick(msg)

	case tea.MouseWheelMsg:
		if next, cmd, handled := m.help.Forward(msg); handled {
			m.help = next
			return m, cmd
		}
		return m.handleMouseWheel(msg)

	case tea.MouseMotionMsg:
		m.sidebar.Tree.SetHoveredIndex(hoverTarget(m, msg))
		return m, nil

	case rowactions.StatusChangedMsg:
		// Up mode does nothing: the store watch turns the file write into a
		// ticket-changed event. Down mode has no stream, so it reloads from disk.
		return m, m.OnPageActivated()
	}
	return m, nil
}

// clampSelected rebuilds the sidebar's entries (see sidebar.clampSelected).
func (m *Model) clampSelected() {
	m.sidebar.ClampSelected(sidebarEnvFor(*m))
}

// afterSidebarToggle runs the sidebar's post-toggle bookkeeping, then re-runs
// the search so its matches follow the new collapse state.
func (m *Model) afterSidebarToggle(toggledID string, cmd tea.Cmd) tea.Cmd {
	cmd = m.sidebar.AfterToggle(sidebarEnvFor(*m), toggledID, cmd)
	if m.search.HasQuery() {
		m.recomputeSearchMatches()
	}
	return cmd
}

// handleSidebarMouseClick selects the sidebar row under a left click,
// mirroring arrow-key navigation with no secondary action (no checkbox
// toggle, no confirm). A click inside the preview panel's bounds instead
// hands focus to it (wheel events then scroll the preview, see
// updateInner's MouseWheelMsg case), without changing the sidebar
// selection. A right click on a ticket or epic row selects it and opens its
// Context menu as "m" does; anywhere else it does nothing.
func (m Model) handleSidebarMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft && mouse.Button != tea.MouseRight {
		return m, nil
	}
	l := m.layout()
	if m.preview.InPreview(mouse, m.width, l.contentHeight()) {
		if mouse.Button == tea.MouseLeft {
			m.preview.FocusPreviewPane()
		}
		return m, nil
	}
	sidebarW, _ := l.splitWidth()
	sidebarH, _ := l.splitHeight(l.contentHeight())
	if l.useStackedLayout() {
		if mouse.Y < 0 || mouse.Y >= sidebarH {
			return m, nil
		}
	} else {
		if mouse.X < 0 || mouse.X >= sidebarW || mouse.Y < 0 || mouse.Y >= l.contentHeight() {
			return m, nil
		}
	}
	bodyLine := mouse.Y - 1
	if mouse.Button == tea.MouseRight {
		if !m.sidebar.SelectRowAt(bodyLine) {
			return m, nil
		}
		m.preview.Focus = preview.FocusSidebar
		return m.handleSuggestedActionsKey()
	}
	m.preview.Focus = preview.FocusSidebar
	if id, toggled := m.sidebar.ClickAt(mouse.X, bodyLine, m.icons()); toggled {
		return m, m.afterSidebarToggle(id, nil)
	}
	return m, nil
}

// handleMouseWheel routes a wheel event to whichever of the sidebar/preview
// panels the cursor is over (ui.HoverHitTest), independent of which pane
// currently holds keyboard focus. A cursor position outside both panels'
// rects is a no-op rather than falling back to the focused pane.
func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	l := m.layout()
	sx, sy, sw, sh := l.sidebarRect()
	px, py, pw, ph := l.previewRect()
	idx, ok := ui.HoverHitTest(mouse.X, mouse.Y, ui.Rect{X: sx, Y: sy, W: sw, H: sh}, ui.Rect{X: px, Y: py, W: pw, H: ph})
	if !ok {
		return m, nil
	}
	if idx == 0 {
		m.sidebar.ScrollWheel(msg)
		return m, nil
	}
	var cmd tea.Cmd
	m.preview.VP, cmd = m.preview.VP.Update(msg)
	return m, cmd
}

func (m Model) scratchDir() string {
	return tabapi.ScratchDir(m.settings.TicketStore, m.worktreeRoot)
}

func (m Model) View() tea.View {
	if !m.ready {
		return ui.NewHoverView("\n  Initializing…")
	}
	content := m.normalView()
	if m.menus.Menu.IsOpen {
		content = m.menus.Menu.Overlay(content, m.menuAnchor(), m.width, m.height)
	} else if m.confirm.IsOpen {
		content = ui.OverlayCenter(content, m.confirm.View(m.width), m.width, m.height)
	}
	if m.watch.IsOpen() {
		content = ui.OverlayCenter(content, m.watch.View(), m.width, m.height)
	}
	if m.help.IsOpen {
		content = ui.OverlayCenter(content, m.help.View(), m.width, m.height)
	}
	if activeSearch, ok := m.activeInputSearch(); ok {
		activeSearch.SetWidth(m.layout().searchOverlayWidth())
		overlay := activeSearch.View()
		y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
		content = ui.OverlayBottomCenter(content, overlay, m.width, y)
	}
	if prefix := m.keys.Prefix(); len(prefix) > 0 {
		hints := ui.ChordBindingsFromHints(m.keys.ChordHints())
		if len(hints) > 0 {
			content = ui.OverlayBottomRight(content, ui.RenderChordOverlay(prefix[0], hints), m.width, m.height)
		}
	}
	return ui.NewHoverView(content)
}

// activeInputSearch returns whichever of the sidebar's or preview's search
// models is mid-input, since only one can be at a time (focus gates which
// one a "/" keypress reaches).
func (m Model) activeInputSearch() (search.Model, bool) {
	if m.search.Mode() == search.SearchModeInput {
		return m.search, true
	}
	if m.preview.Search.Mode() == search.SearchModeInput {
		return m.preview.Search, true
	}
	return search.Model{}, false
}

// normalView lays out the sidebar and preview panels side by side (or
// stacked on narrow terminals), matching the worktrees tab's frame-free
// split layout.
func (m Model) normalView() string {
	l := m.layout()
	sidebarW, previewW := l.splitWidth()
	sidebarH, previewH := l.splitHeight(l.contentHeight())

	sidebarViewportH := l.sidebarViewportHeight()
	sidebarView := renderPanel(sidebarW, sidebarH, "Tickets", m.searchMatchStatus(), m.sidebar.Body(sidebarEnvFor(m), sidebarViewportH, sidebarW-2), m.preview.Focus == preview.FocusSidebar, true)
	previewView := renderPanel(previewW, previewH, "Preview", preview.SearchMatchStatus(m.preview.Search), preview.RenderViewportWithScrollbar(m.preview.VP), m.preview.Focus == preview.FocusPreview, false)

	return m.preview.JoinPanels(m.width, sidebarW, sidebarH, sidebarView, previewView)
}

func renderPanel(width, height int, title, rightTitle string, lines []string, active, sidebar bool) string {
	return ui.RenderPanel(ui.PanelOptionsFor(width, height, title, rightTitle, lines, active, ui.ColorBlue, nil, sidebar))
}

func (m Model) searchMatchStatus() string {
	if m.search.HasQuery() && m.search.MatchesCount() > 0 {
		return fmt.Sprintf("%d/%d matches", m.search.Cursor()+1, m.search.MatchesCount())
	}
	return ""
}
