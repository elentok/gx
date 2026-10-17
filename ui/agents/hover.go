package agents

import tea "charm.land/bubbletea/v2"

// hoveredSession is the session row under the mouse: none while a modal is
// open or off the session list. It reads the stored position rather than a
// stored row, so the sidebar scrolling under a still mouse stays right.
func (m Model) hoveredSession() (int, bool) {
	if m.ModalOpen() {
		return 0, false
	}
	return m.sidebarSessionAt(m.hoverAt.X, m.hoverAt.Y)
}

// HoverChanges moves the hover on a copy, so the model itself is untouched.
func (m Model) HoverChanges(msg tea.MouseMotionMsg) bool {
	before, wasOn := m.hoveredSession()
	m.hoverAt = msg.Mouse()
	after, isOn := m.hoveredSession()
	return before != after || wasOn != isOn
}

// refreshHover runs after every Update: an open modal drops the position, so
// the highlight stays off until the mouse moves again once it closes.
func (m *Model) refreshHover() {
	if m.ModalOpen() {
		m.hoverAt = tea.Mouse{}
	}
}
