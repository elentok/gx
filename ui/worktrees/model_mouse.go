package worktrees

import (
	"github.com/elentok/gx/ui"

	tea "charm.land/bubbletea/v2"
)

// handleMouseWheel routes a wheel event to whichever modal is open (help,
// logs, error), or otherwise to the table or the details preview by hover
// hit-test. A hit on neither (a seam, a border) or a pane with nothing to
// scroll no-ops.
func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.helpModel.Forward(msg); handled {
		m.helpModel = next
		return m, cmd
	}
	if m.mode == modeLogs {
		var cmd tea.Cmd
		m.logsViewport, cmd = m.logsViewport.Update(msg)
		return m, cmd
	}
	if m.mode == modeError {
		var cmd tea.Cmd
		m.errorViewport, cmd = m.errorViewport.Update(msg)
		return m, cmd
	}

	dir, ok := ui.WheelDirection(msg)
	if !ok {
		return m, nil
	}

	mouse := msg.Mouse()
	idx, ok := ui.HoverHitTest(mouse.X, mouse.Y, m.tableRect(), m.previewRect())
	if !ok {
		return m, nil
	}

	if idx == 0 {
		return m.scrollTable(dir)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// scrollTable moves the table cursor by a wheel tick's worth of rows. The
// table has no scroll offset independent of its cursor (rowsView windows
// around it), so moving the cursor is what "scrolling the table" means here.
func (m Model) scrollTable(dir int) (tea.Model, tea.Cmd) {
	if len(m.worktrees) == 0 {
		return m, nil
	}
	prevCursor := m.table.Cursor()
	if dir > 0 {
		m.table.MoveDown(ui.WheelScrollLines)
	} else {
		m.table.MoveUp(ui.WheelScrollLines)
	}
	if m.table.Cursor() == prevCursor {
		return m, nil
	}
	next, cmd := m.refreshAfterCursorMove()
	return next, cmd
}
