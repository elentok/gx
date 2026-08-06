package history

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/filter"
	"github.com/elentok/gx/ui/list"
)

func (m Model) filteredProjects() []claudehistory.Project {
	q := strings.ToLower(strings.TrimSpace(m.projFilter.Query()))
	if q == "" {
		return m.projects
	}
	var out []claudehistory.Project
	for _, p := range m.projects {
		if strings.Contains(strings.ToLower(p.Label), q) || strings.Contains(strings.ToLower(p.Subtitle), q) {
			out = append(out, p)
		}
	}
	return out
}

func (m Model) handleProjectsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.projFilter.InputFocused() || msg.String() == "/" {
		var res filter.Result
		m.projFilter, _, res = m.projFilter.Update(msg)
		if res.QueryChanged {
			m.projList.SetSelected(0, max(len(m.filteredProjects()), 1))
		}
		if res.Handled {
			return m, nil
		}
	}
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "j", "down":
		m.projList.Navigate(1, len(m.filteredProjects()), m.listHeight())
	case "k", "up":
		m.projList.Navigate(-1, len(m.filteredProjects()), m.listHeight())
	case "ctrl+d":
		m.projList.ScrollPage(list.DefaultScroll, len(m.filteredProjects()), m.listHeight())
	case "ctrl+u":
		m.projList.ScrollPage(-list.DefaultScroll, len(m.filteredProjects()), m.listHeight())
	case "enter":
		return m.enterConversations()
	}
	return m, nil
}

func (m Model) enterConversations() (tea.Model, tea.Cmd) {
	items := m.filteredProjects()
	sel := m.projList.Selected()
	if len(items) == 0 || sel >= len(items) {
		return m, nil
	}
	m.page = pageConversations
	m.convProjectDir = items[sel].Dir
	m.convFilter.Clear()
	m.convList = list.Model{}
	m.conversations = nil
	m.convErr = nil
	return m, m.cmdLoadConversations(m.convProjectDir)
}

func (m Model) viewProjects() string {
	items := m.filteredProjects()
	sel := m.projList.Selected()
	start, end := m.projList.VisibleRange(len(items), m.listHeight())
	var lines []string
	for i := start; i < end; i++ {
		line := items[i].Label + "  " + ui.StyleMuted.Render(items[i].Subtitle)
		if i == sel {
			line = ui.RenderRowHighlight(items[i].Label + "  " + items[i].Subtitle)
		}
		lines = append(lines, line)
	}
	if m.projErr != nil {
		lines = []string{ui.StyleWarning.Render("error: " + m.projErr.Error())}
	} else if len(lines) == 0 {
		lines = []string{ui.StyleMuted.Render("no projects match")}
	}
	panel := ui.RenderPanel(ui.PanelOptionsFor(m.w, m.h-2, "Claude Projects", "", lines, true, ui.ColorOrange, ui.ColorOrange, false))
	var top string
	if m.projFilter.IsActive() {
		top = m.projFilter.View() + "\n"
	}
	footer := "  " + ui.StyleHint.Render("/: filter  enter: open  q: quit")
	return top + panel + "\n" + footer
}
