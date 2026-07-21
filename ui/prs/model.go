// Package prs implements the "PRs" tab: a read-only view of the user's
// outgoing GitHub pull requests.
package prs

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
)

var prsTitleColor = ui.ColorMauve

// Model is the top-level PRs tab model.
type Model struct {
	worktreeRoot string
	settings     ui.Settings

	width  int
	height int

	keys keys.Manager
	help help.Model
}

func NewModel(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) Model {
	km := newPRsManager()
	return Model{
		worktreeRoot: worktreeRoot,
		settings:     settings,
		keys:         km,
		help:         help.NewModel(help.BuildSections(km, extraKeys)),
	}
}

// KeyManager exposes the PRs tab's key bindings (used for chord overlays and
// help aggregation by the app shell).
func (m Model) KeyManager() keys.Manager {
	return m.keys
}

func (m Model) IsSplit() bool { return false }

func (m Model) ModalOpen() bool { return m.help.IsOpen }

func (m Model) InputFocused() bool { return m.help.InputFocused() }

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.help.IsOpen {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help, _ = m.help.Update(msg)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "q" {
		return m, nav.Back()
	}

	if match, consumed := m.keys.Process(msg); match != nil {
		return m.dispatchBinding(match.ID, msg)
	} else if consumed {
		return m, nil
	}

	return m, nil
}

func (m Model) View() tea.View {
	out := m.buildMainContent()
	if m.help.IsOpen {
		out = ui.OverlayCenter(out, m.help.View(), m.width, m.height)
	}
	return ui.NewMainView(out)
}

func (m Model) buildMainContent() string {
	lines := []string{ui.StyleMuted.Render("no PRs")}
	panelHeight := max(1, m.height-1)
	panel := ui.RenderPanel(ui.PanelOptionsFor(
		m.width, panelHeight, "PRs", "", lines, true, prsTitleColor, prsTitleColor, false,
	))
	return lipgloss.JoinVertical(lipgloss.Left, panel, prsFooter())
}

func prsFooter() string {
	return "  " + ui.StyleHint.Render("? help")
}
