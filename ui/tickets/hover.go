package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
)

// hoverTarget is the sidebar entry under a motion event: none while a modal
// is open or outside the sidebar panel.
func (m Model) hoverTarget(msg tea.MouseMotionMsg) (int, bool) {
	mouse := msg.Mouse()
	x, y, w, h := m.sidebarRect()
	if m.ModalOpen() || !(ui.Rect{X: x, Y: y, W: w, H: h}).Contains(mouse.X, mouse.Y) {
		return -1, false
	}
	return m.sidebarTree.EntryAtBodyLine(mouse.Y - y - 1)
}

// HoverChanges sets the hover on a copy, so the model itself is untouched.
func (m Model) HoverChanges(msg tea.MouseMotionMsg) bool {
	return m.sidebarTree.SetHoveredIndex(m.hoverTarget(msg))
}

func (m Model) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	m.sidebarTree.SetHoveredIndex(m.hoverTarget(msg))
	return m, nil
}

// refreshHover runs after every Update: an open modal clears the highlight,
// and a wheel scroll moves other rows under the still cursor.
func (m *Model) refreshHover(msg tea.Msg) {
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		m.sidebarTree.SetHoveredIndex(m.hoverTarget(tea.MouseMotionMsg(wheel)))
	} else if m.ModalOpen() {
		m.sidebarTree.SetHoveredIndex(0, false)
	}
}
