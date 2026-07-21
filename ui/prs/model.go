// Package prs implements the "PRs" tab: a read-only view of the user's
// outgoing GitHub pull requests.
package prs

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/nav"
)

var prsTitleColor = ui.ColorMauve

// Model is the top-level PRs tab model.
type Model struct {
	worktreeRoot string
	settings     ui.Settings

	width  int
	height int

	loaded bool
	err    error
	prs    []git.PR
	anyPRs bool
	list   list.Model

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
	return m.cmdLoad()
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

	case prsLoadedMsg:
		m.loaded = true
		m.err = msg.err
		m.prs = msg.prs
		m.anyPRs = msg.anyPRs
		m.list.SetSelected(m.list.Selected(), len(m.prs))
		return m, nil

	case gotoPRMsg:
		return m.handleGotoPR(msg)

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
	panelHeight := max(1, m.height-1)
	panel := ui.RenderPanel(ui.PanelOptionsFor(
		m.width, panelHeight, "PRs", "", m.visibleLines(), true, prsTitleColor, prsTitleColor, false,
	))
	return lipgloss.JoinVertical(lipgloss.Left, panel, prsFooter())
}

// visibleH returns how many PR rows fit in the panel body. Rows render as
// two physical lines (subject + facet line), so this halves the available
// height.
func (m Model) visibleH() int {
	return max(1, (m.height-3)/2)
}

func prsFooter() string {
	return "  " + ui.StyleHint.Render("? help")
}
