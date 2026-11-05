package log

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/splitview"
)

// handleMouseClick focuses whichever split panel a click landed in (see
// splitview.Model.HandleMouseClick), so a click on the detail pane routes
// subsequent wheel events there instead of scrolling the log list.
func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.help.IsOpen || m.commitOps.AmendOpen() {
		return m, nil
	}
	m.split = m.split.HandleMouseClick(msg)
	return m, nil
}

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.help.Forward(msg); handled {
		m.help = next
		return m, cmd
	}
	if m.commitOps.AmendOpen() {
		return m, nil
	}
	if m.commitInfo.open {
		m.commitInfo.wheel(msg, m.width, m.height)
		return m, nil
	}
	var cmd tea.Cmd
	m.commitDetail, m.listPanel, cmd = splitview.RouteWheel(m.split, m.commitDetail, m.listPanel, msg)
	return m, cmd
}
