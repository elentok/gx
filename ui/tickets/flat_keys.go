package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/terminalrun"
)

// newFlatTicketsManager builds the flat ralph-loop TUI's key manager,
// reusing the same binding IDs as ui/tickets' own sidebar (newTicketsManager)
// since the edit-chord/refresh actions are identical — only the
// collapse/expand-epic bindings are dropped, since a flat list has no epic
// rows to collapse.
func newFlatTicketsManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingTicketsDown, Seq: []string{"j"}, Categories: []string{"Navigation"}, Title: "down", Display: "↓/j"},
		{ID: bindingTicketsDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsUp, Seq: []string{"k"}, Categories: []string{"Navigation"}, Title: "up", Display: "↑/k"},
		{ID: bindingTicketsUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsExpand, Seq: []string{"l"}, Categories: []string{"Navigation"}, Title: "focus preview", Display: "l/→"},
		{ID: bindingTicketsExpand, Seq: []string{"right"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsToggle, Seq: []string{"enter"}, Categories: []string{"Navigation"}, Title: "jump to herdr pane"},
		{ID: bindingTicketsRefresh, Seq: []string{"R"}, Categories: []string{"Navigation"}, Title: "refresh"},
		{ID: bindingTicketsEditInPlace, Seq: []string{"e", "e"}, Categories: []string{"Navigation"}, Title: "edit file"},
		{ID: bindingTicketsEditHSplit, Seq: []string{"e", "s"}, Categories: []string{"Navigation"}, Title: "edit file (split)"},
		{ID: bindingTicketsEditVSplit, Seq: []string{"e", "v"}, Categories: []string{"Navigation"}, Title: "edit file (vsplit)"},
		{ID: bindingTicketsEditTab, Seq: []string{"e", "t"}, Categories: []string{"Navigation"}, Title: "edit file (tab)"},
		{ID: bindingTicketsCancelChord, Seq: []string{"e", "esc"}, Categories: []string{}, Title: ""},
	})
}

func (m FlatModel) handleFlatKey(msg tea.KeyPressMsg) (FlatModel, tea.Cmd) {
	if m.focus == flatFocusPreview {
		return m.handleFlatPreviewKey(msg)
	}

	if msg.String() == "q" {
		return m, tea.Quit
	}

	match, consumed := m.keys.Process(msg)
	if !consumed {
		return m, nil
	}
	if match == nil {
		return m, nil // chord in progress
	}

	switch match.ID {
	case bindingTicketsDown:
		m.moveSelection(1)
	case bindingTicketsUp:
		m.moveSelection(-1)
	case bindingTicketsExpand:
		if _, ok := m.selectedTicket(); ok {
			m.focus = flatFocusPreview
		}
	case bindingTicketsToggle:
		if t, ok := m.selectedTicket(); ok {
			if tabID, ok := m.liveTabID(t.Identifier); ok {
				return m, m.cmdFocusTab(tabID)
			}
		}
	case bindingTicketsRefresh:
		return m, m.cmdRefresh()
	case bindingTicketsEditInPlace:
		return m, m.cmdEditSelectedFile(terminalrun.InPlace)
	case bindingTicketsEditHSplit:
		return m, m.cmdEditSelectedFile(terminalrun.HSplit)
	case bindingTicketsEditVSplit:
		return m, m.cmdEditSelectedFile(terminalrun.VSplit)
	case bindingTicketsEditTab:
		return m, m.cmdEditSelectedFile(terminalrun.Tab)
	case bindingTicketsCancelChord:
		return m, nil
	}
	return m, nil
}

func (m *FlatModel) moveSelection(delta int) {
	n := len(m.ordered)
	if n == 0 {
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= n {
		m.selected = n - 1
	}
}
