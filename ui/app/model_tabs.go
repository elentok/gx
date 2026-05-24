package app

import (
	"strings"
	"unicode"

	"github.com/elentok/gx/ui"
	logui "github.com/elentok/gx/ui/log"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/navstate"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) ensureLivePages() {
	for _, kind := range []nav.TabID{nav.TabWorktrees, nav.TabLog, nav.TabStatus} {
		if _, ok := m.livePageByTab[kind]; !ok {
			m.livePageByTab[kind] = livePage{}
		}
	}
}

func (m Model) switchTab(viewState nav.ViewState) (tea.Model, tea.Cmd) {
	prevViewState := m.router.Active()
	tr := m.router.Switch(viewState)
	tabViewState := tr.ViewState

	// Clear model-side stack — tab switch exits the current deep-navigation session.
	m.stack = nil
	m.ensureLivePages()

	currentPage := m.livePageByTab[tabViewState.Tab]

	if currentPage.model == nil || !navstate.SameViewContext(prevViewState.Context(), tabViewState.Context()) {
		currentPage = m.newLivePage(tabViewState)
		currentPage.didInit = true
		m.livePageByTab[tabViewState.Tab] = currentPage
		return m, tea.Batch(tea.ClearScreen, currentPage.model.Init(), m.resizeCurrentCmd(), onPageActivatedCmd(currentPage.model))
	}
	if !currentPage.didInit {
		currentPage.didInit = true
		m.livePageByTab[tabViewState.Tab] = currentPage
		return m, tea.Batch(tea.ClearScreen, currentPage.model.Init(), m.resizeCurrentCmd(), onPageActivatedCmd(currentPage.model))
	}
	if viewState.FocusSubject != "" {
		if logModel, ok := currentPage.model.(logui.Model); ok {
			currentPage.model = logModel.WithPendingFocus(viewState.FocusSubject)
			m.livePageByTab[tabViewState.Tab] = currentPage
		}
	}
	return m, tea.Batch(tea.ClearScreen, m.resizeCurrentCmd(), onPageActivatedCmd(currentPage.model))
}

type pageActivationAware interface {
	OnPageActivated() tea.Cmd
}

func onPageActivatedCmd(model tea.Model) tea.Cmd {
	if activator, ok := model.(pageActivationAware); ok {
		return activator.OnPageActivated()
	}
	return nil
}

func injectTabsIntoFooter(content, tabs string, width int) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}
	if width <= 0 {
		lines[len(lines)-1] = lines[len(lines)-1] + " " + tabs
		return strings.Join(lines, "\n")
	}
	tabs = ansi.Truncate(tabs, width, "")
	tabsW := ansi.StringWidth(tabs)
	if tabsW >= width {
		lines[len(lines)-1] = tabs
		return strings.Join(lines, "\n")
	}
	rightMax := width - tabsW - 1
	if rightMax < 0 {
		rightMax = 0
	}
	rightContent := strings.TrimLeftFunc(lines[len(lines)-1], unicode.IsSpace)
	right := rightContent
	rightW := ansi.StringWidth(rightContent)
	if rightW > rightMax {
		if rightMax <= 0 {
			right = ""
		} else if rightMax == 1 {
			right = "…"
		} else {
			// Keep the tail where compact footer hints live (context/mode/help).
			right = "…" + ansi.TruncateLeft(rightContent, rightW-rightMax+1, "")
		}
	}
	rightW = ansi.StringWidth(right)
	gap := width - tabsW - rightW
	if gap < 1 {
		gap = 1
	}
	lines[len(lines)-1] = tabs + strings.Repeat(" ", gap) + right
	return strings.Join(lines, "\n")
}

func (m *Model) handleShellChordKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	key := msg.String()
	if m.keyPrefix == "g" {
		m.keyPrefix = ""
		switch key {
		case ",":
			next, cmd := m.switchRelativeTab(-1)
			*m = next.(Model)
			return true, cmd
		case ".":
			next, cmd := m.switchRelativeTab(1)
			*m = next.(Model)
			return true, cmd
		case "w":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabWorktrees})
			*m = next.(Model)
			return true, cmd
		case "l":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabLog})
			*m = next.(Model)
			return true, cmd
		case "s":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabStatus})
			*m = next.(Model)
			return true, cmd
		case "c":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabCommit})
			*m = next.(Model)
			return true, cmd
		case "esc":
			return true, nil
		default:
			current := m.activePage()
			replayed, cmd := replayKeys(current.model, tea.KeyPressMsg{Code: 'g', Text: "g"}, msg)
			current.model = replayed
			m.setActivePage(current)
			return true, cmd
		}
	}
	if key == "g" {
		m.keyPrefix = "g"
		return true, nil
	}
	switch key {
	case "1":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabWorktrees})
		*m = next.(Model)
		return true, cmd
	case "2":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabLog})
		*m = next.(Model)
		return true, cmd
	case "3":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabStatus})
		*m = next.(Model)
		return true, cmd
	case "4":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabCommit})
		*m = next.(Model)
		return true, cmd
	}
	return false, nil
}

func replayKeys(model tea.Model, msgs ...tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	current := model
	for _, msg := range msgs {
		next, cmd := current.Update(msg)
		current = next
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return current, tea.Batch(cmds...)
}

func (m Model) newLivePage(viewState nav.ViewState) livePage {
	return livePage{
		model: m.newHistoryEntry(viewState).model,
	}
}

func (m Model) tabsView() string {
	activeTab := m.router.ActiveTab()
	tabs := []tabSpec{
		{label: "worktrees", active: activeTab == nav.TabWorktrees},
		{label: "log", active: activeTab == nav.TabLog},
		{label: "status", active: activeTab == nav.TabStatus},
		{label: "commit", active: activeTab == nav.TabCommit},
	}
	parts := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		parts = append(parts, renderTab(tab))
	}

	return strings.Join(parts, " ")
}

type tabSpec struct {
	label  string
	active bool
}

func renderTab(tab tabSpec) string {
	if tab.active {
		return ui.RenderBadge(tab.label, ui.BadgeVariantOrange, true, true)
	}
	return ui.RenderBadge(tab.label, ui.BadgeVariantSurface, true, true)
}

func orderedTabs() []nav.TabID {
	return []nav.TabID{nav.TabWorktrees, nav.TabLog, nav.TabStatus, nav.TabCommit}
}

func (m Model) switchRelativeTab(delta int) (tea.Model, tea.Cmd) {
	tabs := orderedTabs()
	idx := 0
	activeTab := m.router.ActiveTab()
	for i, kind := range tabs {
		if kind == activeTab {
			idx = i
			break
		}
	}
	next := idx + delta
	if next < 0 {
		next = 0
	}
	if next >= len(tabs) {
		next = len(tabs) - 1
	}
	return m.switchTab(nav.ViewState{Tab: tabs[next]})
}
