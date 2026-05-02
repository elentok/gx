package app

import (
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	activeTabStyle = lipgloss.NewStyle().
			Foreground(ui.ColorDeepBg).
			Background(ui.ColorOrange).
			Bold(true).
			Padding(0, 1)
	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(ui.ColorSubtle).
				Faint(true).
				Padding(0, 1)
)

func (m *Model) ensureTabs() {
	for _, kind := range []nav.RouteKind{nav.RouteWorktrees, nav.RouteLog, nav.RouteStatus} {
		if _, ok := m.tabs[kind]; ok {
			continue
		}
		m.tabs[kind] = m.newTabPage(m.tabStateForRoute(nav.Route{Kind: kind}))
	}
}

func (m Model) switchTab(route nav.Route) (tea.Model, tea.Cmd) {
	tabState := m.tabStateForRoute(route)
	m.ensureTabs()
	m.history = nil
	m.activeTab = tabState.kind
	current, ok := m.tabs[tabState.kind]
	if !ok || !sameTabState(current, tabState) {
		current = m.newTabPage(tabState)
		current.initialized = true
		m.tabs[tabState.kind] = current
		return m, tea.Batch(tea.ClearScreen, current.model.Init(), m.resizeCurrentCmd())
	}
	if !current.initialized {
		current.initialized = true
		m.tabs[tabState.kind] = current
		return m, tea.Batch(tea.ClearScreen, current.model.Init(), m.resizeCurrentCmd())
	}
	return m, tea.Batch(tea.ClearScreen, m.resizeCurrentCmd())
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
	right := ansi.Truncate(lines[len(lines)-1], rightMax, "")
	rightW := ansi.StringWidth(right)
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
		case "w":
			next, cmd := m.switchTab(nav.Route{Kind: nav.RouteWorktrees})
			*m = next.(Model)
			return true, cmd
		case "l":
			next, cmd := m.switchTab(nav.Route{Kind: nav.RouteLog})
			*m = next.(Model)
			return true, cmd
		case "s":
			next, cmd := m.switchTab(nav.Route{Kind: nav.RouteStatus})
			*m = next.(Model)
			return true, cmd
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

func (m Model) tabStateForRoute(route nav.Route) tabPageState {
	tab := tabPageState{kind: tabForRoute(route.Kind)}
	switch tab.kind {
	case nav.RouteLog:
		tab.ref = route.Ref
		tab.worktreeRoot = route.WorktreeRoot
		if strings.TrimSpace(tab.worktreeRoot) == "" {
			tab.worktreeRoot = m.settings.ActiveWorktreePath
		}
	case nav.RouteStatus:
		tab.initialPath = route.InitialPath
		tab.worktreeRoot = route.WorktreeRoot
		if strings.TrimSpace(tab.worktreeRoot) == "" {
			tab.worktreeRoot = m.settings.ActiveWorktreePath
		}
	case nav.RouteWorktrees:
	}
	return tab
}

func tabForRoute(kind nav.RouteKind) nav.RouteKind {
	switch kind {
	case nav.RouteWorktrees, nav.RouteLog, nav.RouteStatus:
		return kind
	case nav.RouteCommit:
		return nav.RouteLog
	default:
		return nav.RouteWorktrees
	}
}

func sameTabState(a, b tabPageState) bool {
	return a.kind == b.kind &&
		a.worktreeRoot == b.worktreeRoot &&
		a.ref == b.ref &&
		a.initialPath == b.initialPath
}

func (m Model) newTabPage(tab tabPageState) tabPageState {
	tab.model = m.newPage(nav.Route{
		Kind:         tab.kind,
		WorktreeRoot: tab.worktreeRoot,
		Ref:          tab.ref,
		InitialPath:  tab.initialPath,
	}).model
	return tab
}

func (m Model) tabsView() string {
	tabs := []tabSpec{
		{label: "worktrees", active: m.activeTab == nav.RouteWorktrees},
		{label: "log", active: m.activeTab == nav.RouteLog},
		{label: "status", active: m.activeTab == nav.RouteStatus},
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
		return activeTabStyle.Render(tab.label)
	}
	return inactiveTabStyle.Render(tab.label)
}
