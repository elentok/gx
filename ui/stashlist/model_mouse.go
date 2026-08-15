package stashlist

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	commitui "github.com/elentok/gx/ui/commit"
	"github.com/elentok/gx/ui/splitview"
)

// handleMouseWheel routes a wheel event by cursor position rather than
// keyboard focus: over the stash list it scrolls the list, over the detail
// pane it's translated into the embedded commit-detail component's own local
// space and forwarded to its (already-correct) diff/filetree hover routing.
// A region with nothing to scroll, or a cursor over neither region, no-ops.
func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.help.IsOpen {
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		return m, cmd
	}

	mouse := msg.Mouse()
	switch m.split.HoverSideAt(mouse.X, mouse.Y) {
	case splitview.HoverDetail:
		col, row, visible := m.split.DetailOrigin()
		if !visible {
			return m, nil
		}
		translated := tea.MouseWheelMsg{X: mouse.X - col, Y: mouse.Y - row, Button: mouse.Button, Mod: mouse.Mod}
		updated, cmd := m.commitDetail.Update(translated)
		m.commitDetail = updated.(commitui.Model)
		return m, cmd

	case splitview.HoverList:
		dir, ok := ui.WheelDirection(msg)
		if !ok {
			return m, nil
		}
		m.stashList = m.stashList.ScrollViewport(dir * ui.WheelScrollLines)
		return m, nil
	}

	return m, nil
}
