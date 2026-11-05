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
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/ui/tree"
	"github.com/elentok/gx/viewmodel"
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
	epics  []tickets.Epic
	// archivedEpics holds the Archived section's tickets.Epic data (ticket
	// 04's lazy load populates it) — deliberately separate from m.epics so
	// epicsLoadedMsg's wholesale replacement of m.epics on every load/
	// auto-refresh tick never wipes it. archivedEpicCount is the cheap
	// up-front count (see cmdLoad) that arrives on every load regardless of
	// whether archivedEpics itself has been populated yet.
	archivedEpics     []tickets.Epic
	archivedEpicCount int
	// archivedLazy drives the Archived section's on-expand load (ticket 04):
	// Expand() returns the load command the first time the section is
	// expanded, Deliver feeds the result back in, and SetCount (called on
	// every epicsLoadedMsg) auto-invalidates the cache if the archive's count
	// changed since it was loaded.
	archivedLazy tree.LazySection[tickets.Epic]

	// sidebarTree owns the sidebar's selection/scroll/collapse state (ticket
	// 02e1): a tree.Model[sidebarNode] built from buildSidebarEntries,
	// replacing the former m.selected/m.scrollOffset/m.collapsedEpics/
	// m.collapsedTickets fields.
	sidebarTree tree.Model[sidebarNode]
	// explicitCollapsed is the sidebar's durable collapse state (ticket 02
	// collapse-state layering): only ever written to by the user's own
	// expand/collapse/toggle keypresses (see handleSidebarTreeKey), never by
	// a declared default or the transient search-match override. Every
	// rebuild re-derives m.sidebarTree's actual collapse map fresh from this
	// plus epics/search state (deriveCollapsedSidebar, via
	// refreshSidebarCollapse) — this map itself is never overwritten by that
	// derived result, so a default or override can never calcify into a
	// permanent entry.
	explicitCollapsed map[string]bool
	// hideDone is the "tc" chord's toggle (ticket 08): when set, done tickets
	// are excluded from buildSidebarEntries()/sidebarBody() navigation and
	// rendering. Epic done/total header counts read epic.Tickets directly
	// (renderEpicRow), so they're unaffected by this filter.
	hideDone bool
	search   search.Model

	// preview backs the preview panel's own focus/scroll/search state
	// (see preview_focus.go and model_preview_focus.go).
	preview preview.Pane

	confirm confirm.Model

	// menus is the "m"-triggered Context menu on a ticket or epic row.
	menus rowactions.Menus

	// watch backs the Watch agent modal (watch_agent.go).
	watch rowactions.WatchModal

	// serverAPI feeds vm (server_mode.go); nil keeps the tab reading the
	// store for good.
	serverAPI tabapi.ServerAPI
	admin     tabapi.AdminAPI
	sessions  tabapi.SessionsAPI
	tickets   tabapi.TicketsAPI
	queue     tabapi.QueueAPI
	reads     tabapi.ReadsAPI
	// link is the link half of the last WithServerState delivery; nothing else
	// writes it (server_link.go).
	link tabapi.ServerLink
	// olderServer: the last snapshot came from a server without rendered
	// status; the tab shows tabapi.OlderServerNotice and no tickets.
	olderServer bool
	vm          viewmodel.State
	// scope is which projects the tab shows; owned by the tab, so no snapshot
	// or event can reset it.
	scope viewmodel.Scope
}

// NewModel creates a new tickets tab model scoped to worktreeRoot's own
// `.scratch/`. extraKeys (the app-wide global bindings) feeds the "?" help
// modal alongside the tab's own bindings, mirroring ui/prs's NewModelWithScope.
func NewModel(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) Model {
	km := newTicketsManager()
	sidebarTree := tree.NewModel[sidebarNode]()
	sidebarTree.SetIsSelectable(func(n sidebarNode) bool {
		return n.kind != nodeLoading && n.kind != nodeLoadError
	})
	scratchDir := tabapi.ScratchDir(settings.TicketStore, worktreeRoot)
	archivedLazy := tree.NewLazySection(0, func() ([]tickets.Epic, error) {
		return tickets.LoadArchived(scratchDir)
	})
	return Model{
		worktreeRoot:      worktreeRoot,
		settings:          settings,
		editor:            rowactions.Editor{WorktreeRoot: worktreeRoot, Settings: settings},
		keys:              km,
		sidebarTree:       sidebarTree,
		help:              help.NewModel(help.BuildSections(km, *sidebarTree.Keys(), extraKeys), settings.About),
		search:            search.NewModel(),
		preview:           preview.NewPane(),
		confirm:           confirm.New(),
		explicitCollapsed: map[string]bool{},
		archivedLazy:      archivedLazy,
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
	if m.readsDisk() {
		return m.cmdLoadDisk()
	}
	return nil // the app shell's stream delivers the state
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
	nm.refreshHover(msg)
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
	if next, cmd, ok := m.updateServer(msg); ok {
		return next, cmd
	}
	if next, cmd, ok := m.updateStoreChanged(msg); ok {
		return next, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.help, _ = m.help.Update(msg)
		m.sidebarTree.SetVisibleHeight(m.sidebarViewportHeight())
		return m, nil

	case epicsLoadedMsg:
		if !m.readsDisk() {
			// A disk read that outlived down mode must not overwrite stream rows.
			return m, nil
		}
		firstLoad := !m.loaded
		m.loaded = true
		m.epics = msg.epics
		m.archivedEpicCount = msg.archivedEpicCount
		m.archivedLazy.SetCount(msg.archivedEpicCount)
		if m.search.HasQuery() {
			m.recomputeSearchMatches()
		}
		m.clampSelected()
		if firstLoad {
			m.selectFirstEpic()
		}
		if msg.err != nil {
			return m, notify.Error("load .scratch/: " + msg.err.Error())
		}
		return m, nil

	case tree.LazyResultMsg[tickets.Epic]:
		if m.archivedLazy.Deliver(msg) {
			m.archivedEpics = m.archivedLazy.Rows()
			m.clampSelected()
		}
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
		return m.handleTicketMenuReady(msg)

	case rowactions.TicketPickMsg:
		return m.handleTicketMenuPick(msg)

	case rowactions.EpicPickMsg:
		return m.handleEpicMenuPick(msg)

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
			return m.handleConfirmUpdate(msg)
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
			return m.handleConfirmMouseUpdate(msg)
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
		return m.handleMouseMotion(msg)

	case rowactions.StatusChangedMsg:
		return m.handleStatusChanged()
	}
	return m, nil
}

// clampSelected rebuilds the sidebar tree's entries from the current
// epics/hideDone/collapse state, e.g. after a collapse hides the rows below
// the selection — SetEntries re-clamps selection to the new entry count.
// An empty-section placeholder is a real tree.Entry row but must never hold
// the cursor, so every rebuild (including the very first, off of
// tree.NewModel's zero-value selection at index 0) nudges off one if the
// rebuilt entries left the selection sitting on one.
func (m *Model) clampSelected() {
	m.refreshSidebarCollapse()
	m.sidebarTree.SetEntries(m.buildSidebarEntries())
	m.sidebarTree.SkipUnselectable(1)
}

// selectFirstEpic puts the cursor on the first epic row, so a freshly loaded
// tab opens on an epic rather than on the section header above it.
func (m *Model) selectFirstEpic() {
	for i, e := range m.sidebarTree.Entries() {
		if e.Value.kind == nodeEpic {
			m.sidebarTree.SetSelectedIndex(i)
			return
		}
	}
}

// refreshSidebarCollapse re-derives m.sidebarTree's collapse map fresh from
// m.explicitCollapsed/m.epics/m.search's query (deriveCollapsedSidebar) and
// hands it to m.sidebarTree — every rebuild path funnels through here (or
// through recomputeSearchMatches, which does the same) so the sidebar's
// three collapse-state read sites (buildSidebarEntries, the section-header
// glyph, isCollapsed/renderEpicRow) always read through the one map that
// was actually handed to the tree.
func (m *Model) refreshSidebarCollapse() {
	m.sidebarTree.SetCollapsedIDs(deriveCollapsedSidebar(m.explicitCollapsed, m.epics, m.archivedEpics, m.search.Query()))
}

// sidebarViewportHeight is the sidebar body's visible line count, matching
// ui.RenderPanel's own bodyH math (PaddingY: 0, minus the header row) so the
// windowing done here lines up with what RenderPanel actually paints.
func (m Model) sidebarViewportHeight() int {
	sidebarH, _ := m.splitHeight(m.contentHeight())
	return max(sidebarH-1, 0)
}

// sidebarBody renders the sidebar panel's body lines. m.sidebarTree.Entries()
// is empty before the first epicsLoadedMsg arrives, and also after a load
// with no epics at all (empty sections are hidden), so RenderOpts.EmptyLine
// covers the pre-load "loading…" state and the zero-epics-after-load
// "no .scratch/ directory found" message keeps its own short-circuit ahead
// of the RenderLines call. That short-circuit only
// applies when there's truly nothing to show: a non-zero archivedEpicCount
// means the Archived section (ticket 04) still has something to render, so
// an empty active `.scratch/` alone must not suppress the whole tree.
func (m Model) sidebarBody(sidebarViewportH, width int) []string {
	if m.olderServer {
		return []string{ui.StyleWarning.Render("  " + tabapi.OlderServerNotice)}
	}
	if m.loaded && len(m.epics) == 0 && m.archivedEpicCount == 0 {
		return []string{ui.StyleMuted.Render("  no .scratch/ directory found")}
	}
	// RenderLines' own height param is an outer-panel height, from which it
	// subtracts 2 internally (see ui/tree/render.go) — the same convention
	// ui/status and ui/commit's own tree.Model callers already rely on. +2
	// here cancels that back out so it renders exactly sidebarViewportH body
	// rows, matching what the (headerless) sidebar panel actually has room for.
	return m.sidebarTree.RenderLines(sidebarViewportH+2, m.sidebarRenderOpts(width))
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
	if m.preview.InPreview(mouse, m.width, m.contentHeight()) {
		if mouse.Button == tea.MouseLeft {
			m.preview.FocusPreviewPane()
		}
		return m, nil
	}
	sidebarW, _ := m.splitWidth()
	sidebarH, _ := m.splitHeight(m.contentHeight())
	if m.useStackedLayout() {
		if mouse.Y < 0 || mouse.Y >= sidebarH {
			return m, nil
		}
	} else {
		if mouse.X < 0 || mouse.X >= sidebarW || mouse.Y < 0 || mouse.Y >= m.contentHeight() {
			return m, nil
		}
	}
	bodyLine := mouse.Y - 1
	if mouse.Button == tea.MouseRight {
		idx, ok := m.sidebarTree.EntryAtBodyLine(bodyLine)
		if !ok {
			return m, nil
		}
		if _, ok := rowFromEntry(m.sidebarTree.Entries()[idx]); !ok {
			return m, nil
		}
		m.preview.Focus = preview.FocusSidebar
		m.sidebarTree.SetSelectedIndex(idx)
		return m.handleSuggestedActionsKey()
	}
	m.preview.Focus = preview.FocusSidebar
	if idx, ok := m.sidebarTree.EntryAtBodyLine(bodyLine); ok {
		entry := m.sidebarTree.Entries()[idx]
		if rowfmt.OnTreeIcon(mouse.X, entry, 0, m.icons()) && m.sidebarTree.ToggleEntry(idx) {
			return m, m.afterSidebarToggle(entry.ID, nil)
		}
		m.sidebarTree.SetSelectedIndex(idx)
	}
	return m, nil
}

// handleMouseWheel routes a wheel event to whichever of the sidebar/preview
// panels the cursor is over (ui.HoverHitTest), independent of which pane
// currently holds keyboard focus. A cursor position outside both panels'
// rects is a no-op rather than falling back to the focused pane.
func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	sx, sy, sw, sh := m.sidebarRect()
	px, py, pw, ph := m.previewRect()
	idx, ok := ui.HoverHitTest(mouse.X, mouse.Y, ui.Rect{X: sx, Y: sy, W: sw, H: sh}, ui.Rect{X: px, Y: py, W: pw, H: ph})
	if !ok {
		return m, nil
	}
	if idx == 0 {
		return m.handleSidebarMouseWheel(msg)
	}
	var cmd tea.Cmd
	m.preview.VP, cmd = m.preview.VP.Update(msg)
	return m, cmd
}

// handleSidebarMouseWheel scrolls the sidebar viewport without moving
// selection, mirroring QueueModel.handleQueueMouseWheel.
func (m Model) handleSidebarMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	dir, ok := ui.WheelDirection(msg)
	if !ok {
		return m, nil
	}
	m.sidebarTree.ScrollViewport(dir * ui.WheelScrollLines)
	return m, nil
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
		overlayW := m.searchOverlayWidth()
		activeSearch.SetWidth(overlayW)
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
	sidebarW, previewW := m.splitWidth()
	h := m.contentHeight()
	sidebarH, previewH := m.splitHeight(h)

	sidebarViewportH := m.sidebarViewportHeight()
	sidebarView := m.renderPanel(sidebarW, sidebarH, "Tickets", m.searchMatchStatus(), m.sidebarBody(sidebarViewportH, sidebarW-2), m.preview.Focus == preview.FocusSidebar, true)
	previewView := m.renderPanel(previewW, previewH, "Preview", preview.SearchMatchStatus(m.preview.Search), preview.RenderViewportWithScrollbar(m.preview.VP), m.preview.Focus == preview.FocusPreview, false)

	return m.preview.JoinPanels(m.width, sidebarW, sidebarH, sidebarView, previewView)
}

func (m Model) renderPanel(width, height int, title, rightTitle string, lines []string, active, sidebar bool) string {
	return ui.RenderPanel(ui.PanelOptionsFor(width, height, title, rightTitle, lines, active, ui.ColorBlue, nil, sidebar))
}

func (m Model) searchMatchStatus() string {
	if m.search.HasQuery() && m.search.MatchesCount() > 0 {
		return fmt.Sprintf("%d/%d matches", m.search.Cursor()+1, m.search.MatchesCount())
	}
	return ""
}

func (m Model) searchOverlayWidth() int {
	max := m.width * 80 / 100
	if search.DESIRED_WIDTH < max {
		return search.DESIRED_WIDTH
	}
	return max
}

func (m Model) useStackedLayout() bool {
	return preview.UseStackedLayout(m.width)
}

func (m Model) splitWidth() (sidebarW, previewW int) {
	return m.preview.PanelWidths(m.width)
}

// splitHeight divides a stacked tickets view evenly between its selection-
// driving list and preview. Wide views remain a full-height side-by-side split.
func (m Model) splitHeight(total int) (sidebarH, previewH int) {
	return m.preview.PanelHeights(m.width, total)
}

// previewRect returns the preview panel's absolute on-screen bounds,
// mirroring normalView's layout math so mouse hit-testing (click-to-focus,
// wheel routing) stays in sync with what's actually rendered.
func (m Model) previewRect() (x, y, w, h int) {
	return m.preview.Rect(m.width, m.contentHeight())
}

// sidebarRect returns the sidebar panel's absolute on-screen bounds,
// mirroring previewRect's layout math so wheel hit-testing stays in sync
// with what's actually rendered.
func (m Model) sidebarRect() (x, y, w, h int) {
	sidebarW, _ := m.splitWidth()
	sidebarH, _ := m.splitHeight(m.contentHeight())
	return 0, 0, sidebarW, sidebarH
}

func (m Model) contentHeight() int {
	return max(m.height, 4)
}
