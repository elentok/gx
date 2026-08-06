package history

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/filter"
	"github.com/elentok/gx/ui/list"
)

func (m Model) filteredConversations() []claudehistory.Conversation {
	q := strings.ToLower(strings.TrimSpace(m.convFilter.Query()))
	if q == "" {
		return m.conversations
	}
	var out []claudehistory.Conversation
	for _, c := range m.conversations {
		if strings.Contains(strings.ToLower(c.Title), q) {
			out = append(out, c)
		}
	}
	return out
}

func (m Model) handleConversationsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.convFilter.InputFocused() || msg.String() == "/" {
		var res filter.Result
		m.convFilter, _, res = m.convFilter.Update(msg)
		if res.QueryChanged {
			m.convList.SetSelected(0, max(len(m.filteredConversations()), 1))
		}
		if res.Handled {
			return m, nil
		}
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.convFilter.IsActive() {
			m.convFilter.Clear()
			return m, nil
		}
		m.page = pageProjects
		return m, nil
	case "j", "down":
		m.convList.Navigate(1, len(m.filteredConversations()), m.listHeight())
	case "k", "up":
		m.convList.Navigate(-1, len(m.filteredConversations()), m.listHeight())
	case "ctrl+d":
		m.convList.ScrollPage(list.DefaultScroll, len(m.filteredConversations()), m.listHeight())
	case "ctrl+u":
		m.convList.ScrollPage(-list.DefaultScroll, len(m.filteredConversations()), m.listHeight())
	}
	return m, nil
}

func (m Model) viewConversations() string {
	items := m.filteredConversations()
	sel := m.convList.Selected()
	start, end := m.convList.VisibleRange(len(items), m.listHeight())
	var lines []string
	for i := start; i < end; i++ {
		rel := ui.RelativeTimeCompact(items[i].LastAccessed)
		line := items[i].Title + "  " + ui.StyleMuted.Render(rel)
		if i == sel {
			line = ui.RenderRowHighlight(items[i].Title + "  " + rel)
		}
		lines = append(lines, line)
	}
	if m.convErr != nil {
		lines = []string{ui.StyleWarning.Render("error: " + m.convErr.Error())}
	} else if len(lines) == 0 {
		lines = []string{ui.StyleMuted.Render("no conversations match")}
	}
	panel := ui.RenderPanel(ui.PanelOptionsFor(m.w, m.h-2, "Conversations", "", lines, true, ui.ColorOrange, ui.ColorOrange, false))
	var top string
	if m.convFilter.IsActive() {
		top = m.convFilter.View() + "\n"
	}
	footer := "  " + ui.StyleHint.Render("/: filter  esc: back")
	return top + panel + "\n" + footer
}
