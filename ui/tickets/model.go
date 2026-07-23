// Package tickets implements the `gx tickets` tab: a sidebar+preview pairing
// (the worktrees archetype per ADR 0009) over the repo's local `.scratch/`
// issue tracker. This ticket only wires the tab shell and empty state; the
// `.scratch/` tree isn't read yet.
package tickets

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// Model is the top-level tickets tab model: an epic/ticket sidebar paired
// with a passive, selection-mirroring preview panel (not a focusable detail
// panel — see CONTEXT.md's panel vocabulary).
type Model struct {
	worktreeRoot string
	settings     ui.Settings
	keyManager   keys.Manager

	width  int
	height int
	ready  bool // true once the first WindowSizeMsg has been received
}

// NewModel creates a new tickets tab model.
func NewModel(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) Model {
	return Model{
		worktreeRoot: worktreeRoot,
		settings:     settings,
		keyManager:   extraKeys,
	}
}

func (m Model) KeyManager() keys.Manager { return m.keyManager }

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil
	}
	return m, nil
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

	sidebarLines := []string{ui.StyleMuted.Render("  no .scratch/ directory found")}
	previewLines := []string{ui.StyleDim.Render("  no ticket selected")}

	sidebarView := m.renderPanel(sidebarW, h, "Tickets", sidebarLines, true, true)
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
