package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
)

// hoverTarget is the sidebar entry under a motion event: none while a modal
// is open or outside the sidebar panel.
func hoverTarget(m Model, msg tea.MouseMotionMsg) (int, bool) {
	mouse := msg.Mouse()
	x, y, w, h := m.layout().sidebarRect()
	if m.ModalOpen() || !(ui.Rect{X: x, Y: y, W: w, H: h}).Contains(mouse.X, mouse.Y) {
		return -1, false
	}
	return m.sidebar.Tree.EntryAtBodyLine(mouse.Y - y - 1)
}

// HoverChanges sets the hover on a copy, so the model itself is untouched.
func (m Model) HoverChanges(msg tea.MouseMotionMsg) bool {
	return m.sidebar.Tree.SetHoveredIndex(hoverTarget(m, msg))
}

// refreshHover runs after every Update: an open modal clears the highlight,
// and a wheel scroll moves other rows under the still cursor.
func refreshHover(m *Model, msg tea.Msg) {
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		m.sidebar.Tree.SetHoveredIndex(hoverTarget(*m, tea.MouseMotionMsg(wheel)))
	} else if m.ModalOpen() {
		m.sidebar.Tree.SetHoveredIndex(0, false)
	}
}
