package stashlist

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/splitview"
)

// handleMouseWheel routes a wheel event by cursor position rather than
// keyboard focus: over the stash list it scrolls the list, over the detail
// pane it's translated into the embedded commit-detail component's own local
// space and forwarded to its (already-correct) diff/filetree hover routing.
// A region with nothing to scroll, or a cursor over neither region, no-ops.
func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.help.Forward(msg); handled {
		m.help = next
		return m, cmd
	}

	var cmd tea.Cmd
	m.commitDetail, m.stashList, cmd = splitview.RouteWheel(m.split, m.commitDetail, m.stashList, msg)
	return m, cmd
}
