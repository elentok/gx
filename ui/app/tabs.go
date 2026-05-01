package app

import (
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"

	"charm.land/lipgloss/v2"
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

func (m Model) tabsView() string {
	tabs := []tabSpec{
		{label: "worktrees", active: m.current.route.Kind == nav.RouteWorktrees},
		{label: "log", active: m.current.route.Kind == nav.RouteLog || m.current.route.Kind == nav.RouteCommit},
		{label: "status", active: m.current.route.Kind == nav.RouteStatus},
	}
	parts := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		parts = append(parts, renderTab(tab))
	}

	line := strings.Join(parts, " ")
	if m.width > 0 {
		w := lipgloss.Width(line)
		if w < m.width {
			line += strings.Repeat(" ", m.width-w)
		}
	}
	return line
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
