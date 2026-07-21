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
	scrollOffset int

	allRepos bool

	keys     keys.Manager
	help     help.Model
	comments commentsPopup
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

func (m Model) ModalOpen() bool { return m.help.IsOpen || m.comments.isOpen }

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

	if m.comments.isOpen {
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			m.comments.handleKey(msg)
			return m, nil
		case commentsLoadedMsg:
			m.comments.loaded(msg.comments, msg.err)
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help, _ = m.help.Update(msg)
		return m.ensureSelectionVisible(), nil

	case openPRsLoadedMsg:
		m.openLoaded = true
		m.err = msg.err
		m.prs = msg.prs
		m.anyPRs = msg.anyPRs
		m.list.SetSelected(m.list.Selected(), m.totalItems())
		return m.ensureSelectionVisible(), nil

	case closedPRsLoadedMsg:
		m.closedLoaded = true
		m.closedPRs = msg.closedPRs
		m.list.SetSelected(m.list.Selected(), m.totalItems())
		return m.ensureSelectionVisible(), nil

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
	if m.comments.isOpen {
		out = ui.OverlayCenter(out, m.comments.view(), m.width, m.height)
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

// viewportH returns how many content lines fit in the panel body, matching
// RenderPanel's own bodyH computation (panelHeight - 1 header row, no
// padding rows in this panel) so the line-based scroll window lines up
// exactly with what RenderPanel will display.
func (m Model) viewportH() int {
	panelHeight := max(1, m.height-1)
	return max(0, panelHeight-1)
}

// totalItems is the size of the combined navigable list: open-PR rows
// followed by closed-PR rows (see issues/10-closed-pr-selectable.md).
func (m Model) totalItems() int {
	return len(m.prs) + len(m.closedPRs)
}

// lineRange is a selectable item's [start, end) line span within the
// unwindowed combined content (open rows, then the closed section).
type lineRange struct {
	start, end int
}

// openLineCount returns the number of lines openListLines would render,
// without doing the actual (width-dependent) rendering — used to locate
// where the closed section starts in line space.
func (m Model) openLineCount() int {
	if !m.openLoaded {
		return 1
	}
	if m.err != nil {
		return len(m.errorLines(m.err))
	}
	if len(m.prs) == 0 {
		return 1
	}
	return len(m.prs) * 2
}

// itemLineRange returns the line span of combined item i: open-PR rows
// render as two lines each starting at line 0, followed by a 2-line spacer
// and closed-section header, then one line per closed PR.
func (m Model) itemLineRange(i int) lineRange {
	if i < len(m.prs) {
		start := i * 2
		return lineRange{start, start + 2}
	}
	start := m.openLineCount() + 2 + (i - len(m.prs))
	return lineRange{start, start + 1}
}

// totalLineCount is the full unwindowed content height: the open section
// plus the 2-line spacer/header plus the closed section.
func (m Model) totalLineCount() int {
	return m.openLineCount() + 2 + m.closedLineCount()
}

func (m Model) closedLineCount() int {
	if !m.closedLoaded {
		return 1
	}
	if len(m.closedPRs) == 0 {
		return 1
	}
	return len(m.closedPRs)
}

// ensureSelectionVisible adjusts scrollOffset minimally so the selected
// item's full line range stays on screen, then clamps it to the content's
// bounds — the single-viewport analogue of ui/list.Model.EnsureSelectionVisible,
// but line-based since open rows (2 lines) and closed rows (1 line) differ in
// height.
func (m Model) ensureSelectionVisible() Model {
	viewportH := m.viewportH()
	total := m.totalLineCount()

	if sel := m.list.Selected(); sel >= 0 && sel < m.totalItems() {
		r := m.itemLineRange(sel)
		if r.start < m.scrollOffset {
			m.scrollOffset = r.start
		}
		if viewportH > 0 && r.end > m.scrollOffset+viewportH {
			m.scrollOffset = r.end - viewportH
		}
	}

	maxOffset := max(0, total-viewportH)
	m.scrollOffset = max(0, min(m.scrollOffset, maxOffset))
	return m
}

func prsFooter() string {
	return "  " + ui.StyleHint.Render("? help")
}
