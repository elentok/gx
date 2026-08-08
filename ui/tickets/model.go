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
)

// focusPane is which of the tickets tab's two panels currently receives key
// input: the sidebar (row navigation/collapse) or the preview (scroll/
// search over the selected row's rendered body).
type focusPane int

const (
	focusSidebar focusPane = iota
	focusPreview
)

// TicketProgressSpinner is the circle-slice pie-fill spinner shared by the
// Tickets tab and Queue tab to indicate an in-progress ticket. Frames
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
	// autoRefreshStarted guards cmdAutoRefresh's self-perpetuating poll loop
	// (auto_refresh.go) against being started more than once per Model
	// instance — every epicsLoadedMsg, including ones the loop itself
	// produces, would otherwise spawn another parallel chain.
	autoRefreshStarted bool

	selected       int
	collapsedEpics map[string]bool
	// collapsedTickets is collapsedEpics' ticket-level counterpart (ticket
	// 09): keyed by Ticket.Path (globally unique, unlike Identifier which
	// restarts numbering per epic), true for a ticket whose children (Parent/
	// Children, ticket 03) are hidden. Unlike collapsedEpics there's no
	// default-collapse pass on load — every ticket with children starts
	// expanded.
	collapsedTickets map[string]bool
	// hideDone is the "tc" chord's toggle (ticket 08): when set, done tickets
	// are excluded from visibleRows()/sidebarLines() navigation and
	// rendering. Epic done/total header counts read epic.Tickets directly
	// (renderEpicRow), so they're unaffected by this filter.
	hideDone bool
	// checked is the Tickets tab's own selection (ticket 04), independent of
	// queue membership since ticket 15's decoupling (ticket 13's design):
	// tickets the user has marked with "space", keyed by Ticket.Path so it
	// survives a reload's re-sorting/index-shuffling. Pressing "i"
	// (handleReplaceQueueKey) pushes this set into the queue and clears it.
	checked map[string]bool
	// checkOrder records when each path joined checked.
	checkOrder map[string]uint64
	// queueStatus is each queued ticket's queue-run status (ticket 11):
	// pending as soon as it's queued, then running/done/errored as execution
	// wiring (tickets 08/09/12) progresses it. Persisted to disk (see
	// queue_state.go) so a restart restores both queue membership and its
	// last-known progress instead of starting empty. Independent of checked
	// since ticket 15 — a ticket can be queued without being checked.
	queueStatus map[string]queueItemStatus
	queueStore  *QueueStore
	// scrollOffset is the sidebar's line-based scroll position (sidebarLines()
	// is windowed to it in normalView), kept following m.selected by
	// ensureSidebarVisible.
	scrollOffset int

	search search.Model

	// previewFocus backs the preview panel's own focus/scroll/search state
	// (see preview_focus.go and model_preview_focus.go); embedded so callers
	// keep reading/writing its fields as m.focus, m.previewVP, etc.
	previewFocus

	// implementAgentMenu, confirm, implementEpic and implementSpinner back the
	// "i"-triggered ralph-loop launch (see implement.go): implementEpic is the
	// name of the epic this tab's own launch (re)started tracking most
	// recently, "" when none — the process-wide "is anything running" check
	// goes through ralphLoopRegistry instead, since that must hold even if
	// this Model gets rebuilt mid-run (e.g. a worktree-context switch).
	// implementingEpics is the actual set this Model is live-tracking (ticket
	// 05): with ralphLoopRegistry now allowing more than one epic in flight
	// process-wide (ticket 03), a resync (OnPageActivated) can hand this
	// Model a second epic's event stream alongside its own launch, so
	// gutter-highlighting/live-row rendering has to check set membership
	// rather than equality against one name.
	implementAgentMenuOpen bool
	implementAgentMenu     components.MenuState
	confirm                confirm.Model
	implementEpic          string
	implementingEpics      map[string]bool
	implementSpinner       spinner.Model

	// Live state is projected from registry snapshots and scoped by epic before
	// ticket identity so concurrent epics cannot collide.
	live            map[string]map[string]liveTicketState
	labelIdentifier map[string]map[string]string
}

// NewModel creates a new tickets tab model scoped to worktreeRoot's own
// `.scratch/`. extraKeys (the app-wide global bindings) feeds the "?" help
// modal alongside the tab's own bindings, mirroring ui/prs's NewModelWithScope.
func NewModel(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) Model {
	return NewModelWithStore(worktreeRoot, settings, extraKeys, LoadQueueStore())
}

func NewModelWithStore(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager, store *QueueStore) Model {
	sp := spinner.New()
	sp.Spinner = TicketProgressSpinner
	snapshot := store.Snapshot()
	km := newTicketsManager()
	return Model{
		worktreeRoot:       worktreeRoot,
		settings:           settings,
		keys:               km,
		help:               help.NewModel(help.BuildSections(km, extraKeys)),
		search:             search.NewModel(),
		previewFocus:       newPreviewFocus(),
		confirm:            confirm.New(),
		implementAgentMenu: newImplementAgentMenu(),
		implementingEpics:  map[string]bool{},
		implementSpinner:   sp,
		live:               map[string]map[string]liveTicketState{},
		labelIdentifier:    map[string]map[string]string{},
		checked:            snapshot.TicketChecked,
		checkOrder:         snapshot.TicketCheckOrder,
		queueStatus:        snapshot.Status,
		queueStore:         store,
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
func (m Model) ModalOpen() bool { return m.help.IsOpen || m.implementAgentMenuOpen || m.confirm.IsOpen }

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
		m.help, _ = m.help.Update(msg)
		m.ensureSidebarVisible()
		return m, nil

	case epicsLoadedMsg:
		if err := autoCheckForkedChildren(m.epics, msg.epics, m.queueStore); err != nil {
			return m, notify.Error("save queue: " + err.Error())
		}
		m.refreshQueueSnapshot()
		m.loaded = true
		m.epics = msg.epics
		m.collapsedEpics = defaultCollapsedEpics(msg.epics, m.collapsedEpics)
		if m.search.HasQuery() {
			m.recomputeSearchMatches()
		}
		m.clampSelected()
		var autoRefreshCmd tea.Cmd
		if !m.autoRefreshStarted {
			m.autoRefreshStarted = true
			autoRefreshCmd = cmdAutoRefresh()
		}
		if msg.err != nil {
			return m, tea.Batch(notify.Error("load .scratch/: "+msg.err.Error()), autoRefreshCmd)
		}
		return m, autoRefreshCmd

	case autoRefreshMsg:
		return m, tea.Batch(m.cmdLoad(), cmdAutoRefresh())

	case editFileFinishedMsg:
		return m.handleEditFileFinished(msg)

	case checkAddConfirmedMsg:
		return m.handleCheckAddConfirmed(msg)

	case replaceQueueConfirmedMsg:
		return m.handleReplaceQueueConfirmed(msg)

	case tea.KeyPressMsg:
		if m.help.IsOpen {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}
		if m.implementAgentMenuOpen {
			return m.handleImplementAgentMenuKey(msg)
		}
		if m.confirm.IsOpen {
			return m.handleConfirmUpdate(msg)
		}
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		if m.implementAgentMenuOpen {
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
		if m.focus == focusPreview {
			var cmd tea.Cmd
			m.previewVP, cmd = m.previewVP.Update(msg)
			return m, cmd
		}
		return m.handleSidebarMouseWheel(msg)

	case implementStartedMsg:
		return m.handleImplementStarted(msg)
	case implementPollMsg:
		return m.handleImplementPoll(msg)
	case implementSyncMsg:
		return m.handleImplementSync(msg)
	case implementFailedMsg:
		return m, notify.Error(msg.err.Error())
	case spinner.TickMsg:
		return m.handleImplementSpinnerTick(msg)
	case reattachSignalsMsg:
		return m.handleReattachSignals(msg)
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
	sidebarH, _ := m.splitHeight(m.contentHeight())
	return max(sidebarH-1, 0)
}

// sidebarLineForSelected returns the selected row's line index and physical
// height within sidebarLines()'s output, accounting for the "── Open epics
// ──"/"── Closed epics ──" section headers (and their "no … epics"
// placeholder lines) interleaved before and between the row blocks.
func (m Model) sidebarLineForSelected() (line, height int, ok bool) {
	if !m.loaded || len(m.epics) == 0 || m.selected < 0 {
		return 0, 0, false
	}
	idxs := make([]int, len(m.epics))
	for i := range m.epics {
		idxs[i] = i
	}
	openIdxs, closedIdxs := splitEpicIndexesBySection(m.epics, idxs)
	openRows := m.rowsForEpicOrder(openIdxs)
	closedRows := m.rowsForEpicOrder(closedIdxs)

	line = 1 // "── Open epics (N) ──"
	if len(openRows) == 0 {
		line++ // "no open epics"
	}
	if m.selected < len(openRows) {
		return m.sidebarLineInRows(line, openRows, m.selected)
	}
	line += m.renderedRowsHeight(openRows)
	line += 2 // blank separator + "── Closed epics (N) ──"
	if len(closedRows) == 0 {
		line++ // "no closed epics"
	}
	idx := m.selected - len(openRows)
	if idx < 0 || idx >= len(closedRows) {
		return 0, 0, false
	}
	return m.sidebarLineInRows(line, closedRows, idx)
}

func (m Model) sidebarLineInRows(line int, rows []row, selected int) (int, int, bool) {
	line += m.renderedRowsHeight(rows[:selected])
	return line, m.renderedRowHeight(rows[selected]), true
}

// sidebarRowForLine returns the visibleRows() index whose rendered lines
// contain targetLine (sidebarLines()' line-numbering), the inverse of
// sidebarLineForSelected.
func (m Model) sidebarRowForLine(targetLine int) (int, bool) {
	if !m.loaded || len(m.epics) == 0 {
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
		line++
	}
	if idx, ok := rowAtLine(m, line, openRows, targetLine); ok {
		return idx, true
	}
	line += m.renderedRowsHeight(openRows)
	line += 2 // blank separator + "── Closed epics (N) ──"
	if len(closedRows) == 0 {
		line++
	}
	if idx, ok := rowAtLine(m, line, closedRows, targetLine); ok {
		return len(openRows) + idx, true
	}
	return 0, false
}

func rowAtLine(m Model, startLine int, rows []row, targetLine int) (int, bool) {
	line := startLine
	for i, r := range rows {
		h := m.renderedRowHeight(r)
		if targetLine >= line && targetLine < line+h {
			return i, true
		}
		line += h
	}
	return 0, false
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
	if bodyLine < 0 {
		return m, nil
	}
	targetLine := bodyLine + m.scrollOffset
	idx, ok := m.sidebarRowForLine(targetLine)
	if !ok {
		return m, nil
	}
	m.focus = focusSidebar
	m.selected = idx
	m.ensureSidebarVisible()
	return m, nil
}

func (m Model) renderedRowsHeight(rows []row) int {
	height := 0
	for _, r := range rows {
		height += m.renderedRowHeight(r)
	}
	return height
}

// renderedRowHeight is always 1 (ticket 06 folded every row's elapsed/token
// metrics onto its single title line).
func (m Model) renderedRowHeight(_ row) int {
	return 1
}

// ensureSidebarVisible adjusts scrollOffset minimally so the selected row's
// line stays within the sidebar's visible window, then clamps it to the
// content's bounds — called after every selection/collapse/resize change so
// the cursor never scrolls off-screen (see notes on the tickets tab's
// scroll-follows-cursor fix).
func (m *Model) ensureSidebarVisible() {
	viewportH := m.sidebarViewportHeight()
	line, rowHeight, ok := m.sidebarLineForSelected()
	if ok {
		if line < m.scrollOffset {
			m.scrollOffset = line
		}
		lastLine := line + rowHeight - 1
		if viewportH > 0 && lastLine >= m.scrollOffset+viewportH {
			m.scrollOffset = lastLine - viewportH + 1
		}
	}
	total := len(m.sidebarLines())
	maxOffset := max(0, total-viewportH)
	m.scrollOffset = max(0, min(m.scrollOffset, maxOffset))
}

// handleSidebarMouseWheel scrolls the sidebar viewport without moving
// selection, mirroring QueueModel.handleQueueMouseWheel.
func (m Model) handleSidebarMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	dir, ok := ui.WheelDirection(msg)
	if !ok {
		return m, nil
	}
	m.scrollOffset += dir * ui.WheelScrollLines
	total := len(m.sidebarLines())
	viewportH := m.sidebarViewportHeight()
	m.scrollOffset = ui.ClampScrollOffset(m.scrollOffset, total, viewportH)
	return m, nil
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
	return scratchDirFor(m.worktreeRoot)
}

// scratchDirFor resolves worktreeRoot's canonical `.scratch` via
// Repo.ScratchRoot(), so a bare-repo checkout's linked worktrees all share
// the same tracker regardless of which one a command runs from. Falls back
// to the plain join when worktreeRoot isn't inside a git repo (e.g. test
// fixtures that use a bare temp dir).
func scratchDirFor(worktreeRoot string) string {
	repo, err := git.FindRepo(worktreeRoot)
	if err != nil {
		return filepath.Join(worktreeRoot, ".scratch")
	}
	return repo.ScratchRoot()
}

func (m Model) View() tea.View {
	if !m.ready {
		return ui.NewMainView("\n  Initializing…")
	}
	content := m.normalView()
	if m.implementAgentMenuOpen {
		content = ui.OverlayCenter(content, m.implementAgentMenuView(), m.width, m.height)
	} else if m.confirm.IsOpen {
		content = ui.OverlayCenter(content, m.confirm.View(m.width), m.width, m.height)
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
	sidebarBody := ui.AppendScrollbar(m.sidebarVisibleLines(sidebarViewportH), sidebarW-2, len(m.sidebarLines()), sidebarViewportH, m.scrollOffset)
	sidebarView := m.renderPanel(sidebarW, sidebarH, "Tickets", m.searchMatchStatus(), sidebarBody, m.focus == focusSidebar, true)
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

func (m Model) contentHeight() int {
	h := m.height - 1 // reserve the footer's keyhints line
	if h < 4 {
		return 4
	}
	return h
}
