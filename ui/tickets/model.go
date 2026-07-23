// Package tickets implements the `gx tickets` tab: a sidebar+preview pairing
// (the worktrees archetype per ADR 0009) over the repo's local `.scratch/`
// issue tracker.
package tickets

import (
	"fmt"
	"path/filepath"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"
)

// focusPane is which of the tickets tab's two panels currently receives key
// input: the sidebar (row navigation/collapse) or the preview (scroll/
// search over the selected row's rendered body).
type focusPane int

const (
	focusSidebar focusPane = iota
	focusPreview
)

// Model is the top-level tickets tab model: an epic/ticket sidebar paired
// with a focusable preview panel that mirrors the sidebar's selection (see
// CONTEXT.md's panel vocabulary) — "l"/"enter" on a ticket row hands focus to
// it for scrolling/searching its body; "h"/"left"/"esc" hands focus back.
type Model struct {
	worktreeRoot string
	settings     ui.Settings
	keys         keys.Manager // this tab's own navigation/collapse bindings

	width  int
	height int
	ready  bool // true once the first WindowSizeMsg has been received

	loaded bool
	epics  []tickets.Epic

	// allRepos is the `gx tickets --all` scope: epics are aggregated across
	// every worktree of the repo (each tagged with Epic.WorktreeName) instead
	// of just m.worktreeRoot's own `.scratch/`, interleaved into the tab's
	// normal single Open/Closed grouping with each epic row labeled by its
	// worktree.
	allRepos bool

	selected       int
	collapsedEpics map[string]bool
	// scrollOffset is the sidebar's line-based scroll position (sidebarLines()
	// is windowed to it in normalView), kept following m.selected by
	// ensureSidebarVisible.
	scrollOffset int

	search search.Model

	// focus, previewVP, previewSelKey and previewSearch back the preview
	// panel's own focus/scroll/search state — see model_preview_focus.go.
	focus         focusPane
	previewVP     viewport.Model
	previewSelKey string // identifies the previewed row, to reset scroll on selection change
	previewSearch search.Model
}

// NewModel creates a new tickets tab model scoped to worktreeRoot's own
// `.scratch/`. extraKeys (the app-wide global bindings) isn't used yet —
// it'll feed a help modal once one exists for this tab, mirroring ui/prs's
// NewModelWithScope.
func NewModel(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) Model {
	return NewModelWithScope(worktreeRoot, settings, extraKeys, false)
}

// NewModelWithScope builds the tickets tab model with an initial scope:
// allRepos true starts it already aggregating every worktree's `.scratch/`
// (the `gx tickets --all` CLI entry point), false starts it scoped to just
// worktreeRoot, mirroring ui/prs's NewModelWithScope.
func NewModelWithScope(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager, allRepos bool) Model {
	_ = extraKeys
	return Model{
		worktreeRoot:  worktreeRoot,
		settings:      settings,
		keys:          newTicketsManager(),
		search:        search.NewModel(),
		previewSearch: search.NewModel(),
		previewVP:     viewport.New(),
		allRepos:      allRepos,
	}
}

func (m Model) KeyManager() keys.Manager { return m.keys }

func (m Model) Init() tea.Cmd {
	return m.cmdLoad()
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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.ensureSidebarVisible()
		return m, nil

	case epicsLoadedMsg:
		m.loaded = true
		m.epics = msg.epics
		m.collapsedEpics = defaultCollapsedEpics(msg.epics)
		if m.search.HasQuery() {
			m.recomputeSearchMatches()
		}
		m.clampSelected()
		if msg.err != nil {
			return m, notify.Error("load .scratch/: " + msg.err.Error())
		}
		return m, nil

	case editFileFinishedMsg:
		return m.handleEditFileFinished(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// clampSelected keeps the selection within the current visible-row range,
// e.g. after a collapse hides the rows below it.
func (m *Model) clampSelected() {
	n := len(m.visibleRows())
	switch {
	case n == 0:
		m.selected = 0
	case m.selected >= n:
		m.selected = n - 1
	case m.selected < 0:
		m.selected = 0
	}
	m.ensureSidebarVisible()
}

// sidebarViewportHeight is the sidebar body's visible line count, matching
// ui.RenderPanel's own bodyH math (PaddingY: 0, minus the header row) so the
// windowing done here lines up with what RenderPanel actually paints.
func (m Model) sidebarViewportHeight() int {
	return max(m.contentHeight()-1, 0)
}

// sidebarLineForSelected returns the selected row's line index within
// sidebarLines()'s output, accounting for the "── Open epics ──"/"── Closed
// epics ──" section headers (and their "no … epics" placeholder lines)
// interleaved before and between the open/closed row blocks. Every row is
// exactly one rendered line, so this is a direct offset computation rather
// than a full render.
func (m Model) sidebarLineForSelected() (int, bool) {
	if !m.loaded || len(m.epics) == 0 || m.selected < 0 {
		return 0, false
	}
	idxs := make([]int, len(m.epics))
	for i := range m.epics {
		idxs[i] = i
	}
	openIdxs, closedIdxs := splitEpicIndexesBySection(m.epics, idxs)
	openRows := m.rowsForEpicOrder(openIdxs)
	closedRows := m.rowsForEpicOrder(closedIdxs)

	line := 1 // "── Open epics (N) ──"
	if len(openRows) == 0 {
		line++ // "no open epics"
	}
	if m.selected < len(openRows) {
		return line + m.selected, true
	}
	line += len(openRows)
	line += 2 // blank separator + "── Closed epics (N) ──"
	if len(closedRows) == 0 {
		line++ // "no closed epics"
	}
	idx := m.selected - len(openRows)
	if idx < 0 || idx >= len(closedRows) {
		return 0, false
	}
	return line + idx, true
}

// ensureSidebarVisible adjusts scrollOffset minimally so the selected row's
// line stays within the sidebar's visible window, then clamps it to the
// content's bounds — called after every selection/collapse/resize change so
// the cursor never scrolls off-screen (see notes on the tickets tab's
// scroll-follows-cursor fix).
func (m *Model) ensureSidebarVisible() {
	viewportH := m.sidebarViewportHeight()
	line, ok := m.sidebarLineForSelected()
	if ok {
		if line < m.scrollOffset {
			m.scrollOffset = line
		}
		if viewportH > 0 && line >= m.scrollOffset+viewportH {
			m.scrollOffset = line - viewportH + 1
		}
	}
	total := len(m.sidebarLines())
	maxOffset := max(0, total-viewportH)
	m.scrollOffset = max(0, min(m.scrollOffset, maxOffset))
}

// sidebarVisibleLines windows sidebarLines() to a single viewportH-line
// scroll position at m.scrollOffset.
func (m Model) sidebarVisibleLines(viewportH int) []string {
	lines := m.sidebarLines()
	start := min(m.scrollOffset, len(lines))
	end := min(start+viewportH, len(lines))
	return lines[start:end]
}

func (m Model) scratchDir() string {
	return filepath.Join(m.worktreeRoot, ".scratch")
}

func (m Model) View() tea.View {
	if !m.ready {
		return ui.NewMainView("\n  Initializing…")
	}
	content := m.normalView()
	if activeSearch, ok := m.activeInputSearch(); ok {
		overlayW := m.searchOverlayWidth()
		activeSearch.SetWidth(overlayW)
		overlay := activeSearch.View()
		y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
		content = ui.OverlayBottomCenter(content, overlay, m.width, y)
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

	sidebarView := m.renderPanel(sidebarW, h, "Tickets", m.searchMatchStatus(), m.sidebarVisibleLines(m.sidebarViewportHeight()), m.focus == focusSidebar, true)
	previewView := m.renderPanel(previewW, h, "Preview", m.previewMatchStatus(), m.previewLines(), m.focus == focusPreview, false)

	if m.useStackedLayout() {
		seam := ui.RenderSeamRow(sidebarW, ui.SeamColor)
		return lipgloss.JoinVertical(lipgloss.Left, sidebarView, seam, previewView)
	}
	seam := ui.RenderSeamColumn(h, ui.SeamColor)
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, seam, previewView)
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
	return m.width <= 100
}

func (m Model) splitWidth() (sidebarW, previewW int) {
	if m.useStackedLayout() {
		return m.width, m.width
	}
	width := m.width - 1
	sidebarW = int(float64(width) * 0.55)
	previewW = width - sidebarW
	return
}

func (m Model) contentHeight() int {
	h := m.height
	if m.useStackedLayout() {
		h -= 1
	}
	if h < 4 {
		return 4
	}
	return h
}
