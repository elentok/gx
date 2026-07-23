// Package tickets implements the `gx tickets` tab: a sidebar+preview pairing
// (the worktrees archetype per ADR 0009) over the repo's local `.scratch/`
// issue tracker.
package tickets

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

// Model is the top-level tickets tab model: an epic/ticket sidebar paired
// with a passive, selection-mirroring preview panel (not a focusable detail
// panel — see CONTEXT.md's panel vocabulary).
type Model struct {
	worktreeRoot string
	settings     ui.Settings
	keys         keys.Manager // this tab's own navigation/collapse bindings

	width  int
	height int
	ready  bool // true once the first WindowSizeMsg has been received

	loaded bool
	epics  []tickets.Epic

	selected       int
	collapsedEpics map[string]bool
}

// NewModel creates a new tickets tab model. extraKeys (the app-wide global
// bindings) isn't used yet — it'll feed a help modal once one exists for
// this tab, mirroring ui/prs's NewModelWithScope.
func NewModel(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) Model {
	_ = extraKeys
	return Model{
		worktreeRoot: worktreeRoot,
		settings:     settings,
		keys:         newTicketsManager(),
	}
}

func (m Model) KeyManager() keys.Manager { return m.keys }

func (m Model) Init() tea.Cmd {
	return m.cmdLoad()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil

	case epicsLoadedMsg:
		m.loaded = true
		m.epics = msg.epics
		m.collapsedEpics = defaultCollapsedEpics(msg.epics)
		m.clampSelected()
		if msg.err != nil {
			return m, notify.Error("load .scratch/: " + msg.err.Error())
		}
		return m, nil

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
}

func (m Model) scratchDir() string {
	return filepath.Join(m.worktreeRoot, ".scratch")
}

func (m Model) View() tea.View {
	if !m.ready {
		return ui.NewMainView("\n  Initializing…")
	}
	return ui.NewMainView(m.normalView())
}

// normalView lays out the sidebar and preview panels side by side (or
// stacked on narrow terminals), matching the worktrees tab's frame-free
// split layout.
func (m Model) normalView() string {
	sidebarW, previewW := m.splitWidth()
	h := m.contentHeight()

	previewLines := []string{ui.StyleDim.Render("  no ticket selected")}

	sidebarView := m.renderPanel(sidebarW, h, "Tickets", m.sidebarLines(), true, true)
	previewView := m.renderPanel(previewW, h, "Preview", previewLines, false, false)

	if m.useStackedLayout() {
		seam := ui.RenderSeamRow(sidebarW, ui.SeamColor)
		return lipgloss.JoinVertical(lipgloss.Left, sidebarView, seam, previewView)
	}
	seam := ui.RenderSeamColumn(h, ui.SeamColor)
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, seam, previewView)
}

func (m Model) renderPanel(width, height int, title string, lines []string, active, sidebar bool) string {
	return ui.RenderPanel(ui.PanelOptionsFor(width, height, title, "", lines, active, ui.ColorBlue, nil, sidebar))
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
