package tickets

import (
	tea "charm.land/bubbletea/v2"
)

func (m QueueModel) handleQueueConfirmUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.Update(msg)
	m.confirm = next
	return m, cmd
}

func (m QueueModel) handleQueueConfirmMouseUpdate(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.UpdateMouse(msg, m.width, m.width, m.height)
	m.confirm = next
	return m, cmd
}

// checkedPaths lists every currently checked ticket path, for the "C" clear
// keymap's confirmation prompt and its accepted clear-all.
func (m QueueModel) checkedPaths() []string {
	paths := make([]string, 0, len(m.checked))
	for path := range m.checked {
		paths = append(paths, path)
	}
	return paths
}

// doneCheckedPaths lists every checked ticket path whose status renders as
// done, for the "c" clear-complete keymap.
func (m QueueModel) doneCheckedPaths() []string {
	var paths []string
	for _, epic := range m.epics {
		for _, t := range epic.Tickets {
			if m.checked[t.Path] && epic.RenderedStatus(t).Terminal() {
				paths = append(paths, t.Path)
			}
		}
	}
	return paths
}
