package log

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
)

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.help.IsOpen || m.amendConfirm.IsOpen {
		return m, nil
	}
	dir, ok := ui.WheelDirection(msg)
	if !ok {
		return m, nil
	}
	m.listPanel = m.listPanel.ScrollViewport(dir * ui.WheelScrollLines)
	return m, nil
}
