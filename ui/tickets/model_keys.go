package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/terminalrun"
)

const (
	bindingTicketsDown        keys.BindingID = "down"
	bindingTicketsUp          keys.BindingID = "up"
	bindingTicketsCollapse    keys.BindingID = "collapse"
	bindingTicketsExpand      keys.BindingID = "expand"
	bindingTicketsToggle      keys.BindingID = "toggle"
	bindingTicketsEditInPlace keys.BindingID = "edit"
	bindingTicketsEditHSplit  keys.BindingID = "edit-hsplit"
	bindingTicketsEditVSplit  keys.BindingID = "edit-vsplit"
	bindingTicketsEditTab     keys.BindingID = "edit-tab"
	bindingTicketsCancelChord keys.BindingID = "cancel-chord"
)

// newTicketsManager builds the key manager for the tickets tab: plain
// up/down navigation plus ui/filetree's collapse/expand/toggle bindings,
// reused rather than reinvented (h/left collapse, l/right expand, enter
// toggles a selected epic row).
func newTicketsManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingTicketsDown, Seq: []string{"j"}, Categories: []string{"Navigation"}, Title: "down", Display: "↓/j"},
		{ID: bindingTicketsDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsUp, Seq: []string{"k"}, Categories: []string{"Navigation"}, Title: "up", Display: "↑/k"},
		{ID: bindingTicketsUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsCollapse, Seq: []string{"h"}, Categories: []string{"Navigation"}, Title: "collapse epic", Display: "h/←"},
		{ID: bindingTicketsCollapse, Seq: []string{"left"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsExpand, Seq: []string{"l"}, Categories: []string{"Navigation"}, Title: "expand epic", Display: "l/→"},
		{ID: bindingTicketsExpand, Seq: []string{"right"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsToggle, Seq: []string{"enter"}, Categories: []string{"Navigation"}, Title: "toggle epic"},
		// e-prefix chords: edit the selected row's underlying file, reusing
		// the same launch-mode plumbing every other tab's edit-chord uses.
		{ID: bindingTicketsEditInPlace, Seq: []string{"e", "e"}, Categories: []string{"Navigation"}, Title: "edit file"},
		{ID: bindingTicketsEditHSplit, Seq: []string{"e", "s"}, Categories: []string{"Navigation"}, Title: "edit file (split)"},
		{ID: bindingTicketsEditVSplit, Seq: []string{"e", "v"}, Categories: []string{"Navigation"}, Title: "edit file (vsplit)"},
		{ID: bindingTicketsEditTab, Seq: []string{"e", "t"}, Categories: []string{"Navigation"}, Title: "edit file (tab)"},
		{ID: bindingTicketsCancelChord, Seq: []string{"e", "esc"}, Categories: []string{}, Title: ""},
	})
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if nextSearch, cmd, result := m.search.Update(msg); result.Handled {
		m.search = nextSearch
		if result.QueryChanged {
			m.recomputeSearchMatches()
		}
		if result.QueryChanged || result.CursorChanged {
			m.jumpToCurrentMatch()
		}
		return m, cmd
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
	case bindingTicketsCollapse:
		m.collapseSelectedEpic()
	case bindingTicketsExpand:
		m.expandSelectedEpic()
	case bindingTicketsToggle:
		m.toggleSelectedEpic()
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

func (m *Model) moveSelection(delta int) {
	n := len(m.visibleRows())
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

// selectedRow returns the row currently under the selection, if any.
func (m Model) selectedRow() (row, bool) {
	rows := m.visibleRows()
	if m.selected < 0 || m.selected >= len(rows) {
		return row{}, false
	}
	return rows[m.selected], true
}

func (m *Model) collapseSelectedEpic() {
	r, ok := m.selectedRow()
	if !ok || !r.isEpic() || m.isCollapsed(m.epics[r.epicIdx]) {
		return
	}
	m.setCollapsed(r.epicIdx, true)
}

func (m *Model) expandSelectedEpic() {
	r, ok := m.selectedRow()
	if !ok || !r.isEpic() || !m.isCollapsed(m.epics[r.epicIdx]) {
		return
	}
	m.setCollapsed(r.epicIdx, false)
}

func (m *Model) toggleSelectedEpic() {
	r, ok := m.selectedRow()
	if !ok || !r.isEpic() {
		return
	}
	m.setCollapsed(r.epicIdx, !m.isCollapsed(m.epics[r.epicIdx]))
}

// setCollapsed sets the collapse state for the epic at epicIdx and
// re-clamps the selection, since collapsing hides rows below it.
func (m *Model) setCollapsed(epicIdx int, collapsed bool) {
	path := m.epics[epicIdx].Path
	if m.collapsedEpics == nil {
		m.collapsedEpics = map[string]bool{}
	}
	if collapsed {
		m.collapsedEpics[path] = true
	} else {
		delete(m.collapsedEpics, path)
	}
	// Collapsing/expanding reshuffles visibleRows(), which search matches
	// index into by position — recompute so they stay aligned.
	if m.search.HasQuery() {
		m.recomputeSearchMatches()
	}
	m.clampSelected()
}
