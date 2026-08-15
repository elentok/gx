package log

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/commit"
)

// handleMouseClick focuses whichever split panel a click landed in (see
// splitview.Model.HandleMouseClick), so a click on the detail pane routes
// subsequent wheel events there instead of scrolling the log list.
func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.help.IsOpen || m.amendConfirm.IsOpen {
		return m, nil
	}
	m.split = m.split.HandleMouseClick(msg)
	return m, nil
}

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.help.IsOpen {
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		return m, cmd
	}
	if m.amendConfirm.IsOpen {
		return m, nil
	}
	if m.split.IsSplit() && m.split.IsDetailFocused() {
		col, row, visible := m.split.DetailOrigin()
		if !visible {
			return m, nil
		}
		mouse := msg.Mouse()
		translated := tea.MouseWheelMsg{X: mouse.X - col, Y: mouse.Y - row, Button: mouse.Button, Mod: mouse.Mod}
		updated, cmd := m.commitDetail.Update(translated)
		m.commitDetail = updated.(commit.Model)
		return m, cmd
	}
	dir, ok := ui.WheelDirection(msg)
	if !ok {
		return m, nil
	}
	m.listPanel = m.listPanel.ScrollViewport(dir * ui.WheelScrollLines)
	return m, nil
}
