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

	openLoaded   bool
	closedLoaded bool
	err          error
	prs          []git.PR
	anyPRs       bool
	closedPRs    []git.ClosedPR
	list         list.Model

	allRepos bool

	keys keys.Manager
	help help.Model
}

func NewModel(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) Model {
	return NewModelWithScope(worktreeRoot, settings, extraKeys, false)
}

// NewModelWithScope builds the PRs tab model with an initial repo scope:
// allRepos true starts it already scoped to all repos (the `gx prs --all`
// CLI entry point), false starts it current-repo scoped.
func NewModelWithScope(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager, allRepos bool) Model {
	km := newPRsManager()
	return Model{
		worktreeRoot: worktreeRoot,
		settings:     settings,
		keys:         km,
		help:         help.NewModel(help.BuildSections(km, extraKeys)),
		allRepos:     allRepos,
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

// OnPageActivated triggers a non-blocking background refetch every time the
// PRs tab is switched into, independent of the git-mutation reload gate
// (which this tab intentionally does not implement — see AutoReload absence).
func (m Model) OnPageActivated() tea.Cmd {
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

	case openPRsLoadedMsg:
		m.openLoaded = true
		m.err = msg.err
		m.prs = msg.prs
		m.anyPRs = msg.anyPRs
		m.list.SetSelected(m.list.Selected(), m.totalItems())
		return m, nil

	case closedPRsLoadedMsg:
		m.closedLoaded = true
		m.closedPRs = msg.closedPRs
		m.list.SetSelected(m.list.Selected(), m.totalItems())
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
	subtitle := ""
	if m.allRepos {
		subtitle = "all repos"
	}
	panel := ui.RenderPanel(ui.PanelOptionsFor(
		m.width, panelHeight, "PRs", subtitle, m.visibleLines(), true, prsTitleColor, prsTitleColor, false,
	))
	return lipgloss.JoinVertical(lipgloss.Left, panel, prsFooter())
}

// visibleH returns how many PR rows fit in the panel body. Rows render as
// two physical lines (subject + facet line), so this halves the available
// height.
func (m Model) visibleH() int {
	return max(1, (m.height-3)/2)
}

// totalItems is the size of the combined navigable list: open-PR rows
// followed by closed-PR rows (see issues/10-closed-pr-selectable.md). Closed
// rows always render in full below the open list rather than scrolling
// within visibleH, so scroll-viewport math (EnsureSelectionVisible/
// VisibleRange) stays scoped to len(m.prs) — only the selection's clamp
// range widens to cover both sections.
func (m Model) totalItems() int {
	return len(m.prs) + len(m.closedPRs)
}

func prsFooter() string {
	return "  " + ui.StyleHint.Render("? help")
}
