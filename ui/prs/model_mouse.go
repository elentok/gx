package prs

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
)

// handleMouseWheel routes a wheel event to whichever region is on top: the
// help modal while it's open, otherwise the PR list. There's no
// detail/preview split to disambiguate against (see issues/11), so this is
// the tab's only wheel target besides the modal.
func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.comments.isOpen {
		m.comments.handleWheel(msg)
		return m, nil
	}

	if m.help.IsOpen {
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		return m, cmd
	}

	dir, ok := ui.WheelDirection(msg)
	if !ok {
		return m, nil
	}

	lines, _ := m.combinedContent()
	m.scrollOffset = ui.ClampScrollOffset(m.scrollOffset+dir*ui.WheelScrollLines, len(lines), m.viewportH())
	return m, nil
}
