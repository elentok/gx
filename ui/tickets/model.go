// Package tickets implements the `gx tickets` tab: a sidebar+preview pairing
// (the worktrees archetype per ADR 0009) over the repo's local `.scratch/`
// issue tracker.
package tickets

import (
	"fmt"
	"path/filepath"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tree"
	"github.com/elentok/gx/viewmodel"
)

// focusPane is which of the tickets tab's two panels currently receives key
// input: the sidebar (row navigation/collapse) or the preview (scroll/
// search over the selected row's rendered body).
type focusPane int

const (
	focusSidebar focusPane = iota
	focusPreview
)

// TicketProgressSpinner is the circle-slice pie-fill spinner the Queue tab
// uses to indicate an in-progress ticket. Frames
// fill 1/8 to full then drain back down (rather than cutting straight from
// full back to 1/8) so the loop point has no visible jump.
var TicketProgressSpinner = spinner.Spinner{
	Frames: []string{
		"\U000F0A9E", "\U000F0A9F", "\U000F0AA0", "\U000F0AA1",
		"\U000F0AA2", "\U000F0AA3", "\U000F0AA4", "\U000F0AA5",
		"\U000F0AA4", "\U000F0AA3", "\U000F0AA2", "\U000F0AA1",
		"\U000F0AA0", "\U000F0A9F",
	},
	FPS: spinner.Dot.FPS,
}

// Model is the top-level tickets tab model: an epic/ticket sidebar paired
// with a focusable preview panel that mirrors the sidebar's selection (see
// CONTEXT.md's panel vocabulary) — "l"/"enter" on a ticket row hands focus to
// it for scrolling/searching its body; "h"/"left"/"esc" hands focus back.
type Model struct {
	worktreeRoot string
	settings     ui.Settings
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
	// checked is the Tickets tab's own selection (ticket 04), independent of
	// queue membership: tickets the user has marked with "space", keyed by
	// Ticket.Path so it survives a reload's re-sorting/index-shuffling. Held
	// in memory only (see setPathsChecked).
	checked map[string]bool

	search search.Model

	// previewFocus backs the preview panel's own focus/scroll/search state
	// (see preview_focus.go and model_preview_focus.go); embedded so callers
	// keep reading/writing its fields as m.focus, m.previewVP, etc.
	previewFocus

	confirm confirm.Model

	// statusMenuOpen/statusMenu back the "s"-triggered change-status menu (see
	// status_menu.go): built fresh from the selected row each time "s" opens it.
	statusMenuOpen bool
	statusMenu     components.MenuState

	// actionsMenu backs the "m"-triggered suggested-actions menu (see
	// actions_menu.go/suggested_actions.go): built fresh from the selected
	// row's rendered status each time "m" opens it.
	actionsMenu actionsMenuModel

	// watch backs the Watch agent modal (watch_agent.go).
	watch watchModal

	// serverAPI feeds vm (server_mode.go); nil keeps the tab reading the
	// store for good.
	serverAPI ServerAPI
	// ticketStore locates the ticket files "Answer…" edits directly in server mode.
	ticketStore string
	// serverLink is how the TUI currently reaches the server (server_link.go).
	serverLink ServerLink
	vm         viewmodel.State
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
		return n.kind != nodeEmpty && n.kind != nodeLoading && n.kind != nodeLoadError
	})
	scratchDir := scratchDirFor(worktreeRoot)
	archivedLazy := tree.NewLazySection(0, func() ([]tickets.Epic, error) {
		return tickets.LoadArchived(scratchDir)
	})
	return Model{
		worktreeRoot:      worktreeRoot,
		settings:          settings,
		keys:              km,
		sidebarTree:       sidebarTree,
		help:              help.NewModel(help.BuildSections(km, *sidebarTree.Keys(), extraKeys)),
		search:            search.NewModel(),
		previewFocus:      newPreviewFocus(),
		confirm:           confirm.New(),
		checked:           map[string]bool{},
		explicitCollapsed: map[string]bool{},
		archivedLazy:      archivedLazy,
	}
}

func (m Model) KeyManager() keys.Manager { return m.keys }

// InputFocused reports whether either search box is mid-input, so the app
// shell's digit-based tab-jump mnemonics (see ui/app's inputFocuser
// duck-type) stay routed to the search query instead of switching tabs.
func (m Model) InputFocused() bool {
	if m.help.InputFocused() {
		return true
	}
	_, ok := m.activeInputSearch()
	return ok
}

// ModalOpen reports whether one of the tab's launch dialogs is open, so the app
// shell (see ui/app's modalOpener duck-type) blocks tab-switch keys and
// routes them here instead while it's up.
func (m Model) ModalOpen() bool {
	return m.help.IsOpen || m.statusMenuOpen || m.actionsMenu.IsOpen || m.confirm.IsOpen || m.watch.open
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
	nm.syncPreviewViewport()
	return nm, cmd
}

func (m Model) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, ok := m.updateServer(msg); ok {
		return next, cmd
	}
	if next, cmd, ok := m.updateServerLink(msg); ok {
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
		m.autoCheckForkedChildren(msg.epics)
		m.loaded = true
		m.epics = msg.epics
		m.archivedEpicCount = msg.archivedEpicCount
		m.archivedLazy.SetCount(msg.archivedEpicCount)
		if m.search.HasQuery() {
			m.recomputeSearchMatches()
		}
		m.clampSelected()
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

	case editFileFinishedMsg:
		return m.handleEditFileFinished(msg)

	case answerEditorFinishedMsg:
		var cmd tea.Cmd
		m.confirm, cmd = handleAnswerEditorFinished(m.confirm, msg, func() tea.Msg { return statusChangedMsg{} })
		return m, cmd

	case checkAddConfirmedMsg:
		return m.handleCheckAddConfirmed(msg)

	case watchLoadedMsg:
		m.watch = m.watch.Open(msg.title, msg.lines, m.width, m.height)
		return m, nil

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
		if m.statusMenuOpen {
			return m.handleStatusMenuKey(msg)
		}
		if m.actionsMenu.IsOpen {
			return m.handleActionsMenuKey(msg)
		}
		if m.confirm.IsOpen {
			return m.handleConfirmUpdate(msg)
		}
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		if m.statusMenuOpen || m.actionsMenu.IsOpen || m.watch.open {
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

	case statusChangedMsg:
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
// is only ever empty before the first epicsLoadedMsg arrives (the root list
// is 2 fixed section entries regardless of len(m.epics), once clampSelected
// has run at least once), so RenderOpts.EmptyLine covers
// the pre-load "loading…" state on its own. It cannot reproduce the
// zero-epics-after-load "no .scratch/ directory found" message though — that
// would otherwise render as two real, empty section headers plus their
// nodeEmpty placeholders, a different visual — so that state keeps its own
// short-circuit ahead of the RenderLines call. That short-circuit only
// applies when there's truly nothing to show: a non-zero archivedEpicCount
// means the Archived section (ticket 04) still has something to render, so
// an empty active `.scratch/` alone must not suppress the whole tree.
func (m Model) sidebarBody(sidebarViewportH, width int) []string {
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
// selection.
func (m Model) handleSidebarMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	if m.previewFocus.clickToFocus(mouse, m.width, m.contentHeight()) {
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
	m.focus = focusSidebar
	m.sidebarTree.SelectAtBodyLine(bodyLine)
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
	m.previewVP, cmd = m.previewVP.Update(msg)
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
	return scratchDirFor(m.worktreeRoot)
}

// scratchDirFor resolves worktreeRoot's repo to its project directory in the
// ticket store, so every linked worktree shares one tracker. Falls back to
// the plain join when worktreeRoot isn't inside a git repo (e.g. test
// fixtures that use a bare temp dir). A repo with no store project gets a
// path that doesn't exist, which renders the empty state rather than reading
// the legacy `.scratch`.
func scratchDirFor(worktreeRoot string) string {
	if _, err := git.FindRepo(worktreeRoot); err != nil {
		return filepath.Join(worktreeRoot, ".scratch")
	}
	root, err := tickets.RootFor(worktreeRoot)
	if err != nil {
		return filepath.Join(worktreeRoot, ".gx-no-ticket-project")
	}
	return root
}

func (m Model) View() tea.View {
	if !m.ready {
		return ui.NewMainView("\n  Initializing…")
	}
	content := m.normalView()
	if m.statusMenuOpen {
		content = ui.OverlayCenter(content, m.statusMenuView(), m.width, m.height)
	} else if m.actionsMenu.IsOpen {
		content = ui.OverlayCenter(content, m.actionsMenu.View(), m.width, m.height)
	} else if m.confirm.IsOpen {
		content = ui.OverlayCenter(content, m.confirm.View(m.width), m.width, m.height)
	}
	if m.watch.open {
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
	return ui.NewMainView(content)
}

// activeInputSearch returns whichever of the sidebar's or preview's search
// models is mid-input, since only one can be at a time (focus gates which
// one a "/" keypress reaches).
func (m Model) activeInputSearch() (search.Model, bool) {
	if m.search.Mode() == search.SearchModeInput {
		return m.search, true
	}
	if m.previewSearch.Mode() == search.SearchModeInput {
		return m.previewSearch, true
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
	sidebarView := m.renderPanel(sidebarW, sidebarH, "Tickets", m.searchMatchStatus(), m.sidebarBody(sidebarViewportH, sidebarW-2), m.focus == focusSidebar, true)
	previewView := m.renderPanel(previewW, previewH, "Preview", m.previewMatchStatus(), m.previewLines(), m.focus == focusPreview, false)

	var body string
	if m.useStackedLayout() {
		seam := ui.RenderSeamRow(sidebarW, ui.SeamColor)
		body = lipgloss.JoinVertical(lipgloss.Left, sidebarView, seam, previewView)
	} else {
		seam := ui.RenderSeamColumn(sidebarH, ui.SeamColor)
		body = lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, seam, previewView)
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, m.footerView())
}

// footerView reserves the plain keyhints line every other ui tab uses
// (".ai/index.md": no keymaps on the statusbar, only "? help"), so the
// app shell's tab bar has a dedicated row to merge into instead of
// overwriting the panels' own bottom border row.
func (m Model) footerView() string {
	return "  " + ui.StyleHint.Render("? help")
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
	return useStackedLayout(m.width)
}

func (m Model) splitWidth() (sidebarW, previewW int) {
	return splitPanelWidth(m.width)
}

// splitHeight divides a stacked tickets view evenly between its selection-
// driving list and preview. Wide views remain a full-height side-by-side split.
func (m Model) splitHeight(total int) (sidebarH, previewH int) {
	return splitPanelHeight(m.width, total)
}

// useStackedLayout, splitPanelWidth and splitPanelHeight are free functions
// (rather than Model methods) so the Queue tab's own list+preview split
// (queue_preview.go) can share the exact same layout math instead of
// re-deriving it - both tabs' panels should size identically at a given
// terminal width.
func useStackedLayout(width int) bool {
	return width <= 100
}

func splitPanelWidth(width int) (sidebarW, previewW int) {
	if useStackedLayout(width) {
		return width, width
	}
	w := width - 1
	sidebarW = w / 2
	previewW = w - sidebarW
	return
}

func splitPanelHeight(width, total int) (sidebarH, previewH int) {
	if !useStackedLayout(width) {
		return total, total
	}
	total-- // seam row
	sidebarH = total / 2
	previewH = total - sidebarH
	return
}

// previewRect returns the preview panel's absolute on-screen bounds,
// mirroring normalView's layout math so mouse hit-testing (click-to-focus,
// wheel routing) stays in sync with what's actually rendered.
func (m Model) previewRect() (x, y, w, h int) {
	return previewRect(m.width, m.contentHeight())
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
	h := m.height - 1 // reserve the footer's keyhints line
	if h < 4 {
		return 4
	}
	return h
}
