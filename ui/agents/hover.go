package agents

import tea "charm.land/bubbletea/v2"

// hoveredSession is the session row under the mouse: none while a modal is
// open or off the session list. It reads the stored position rather than a
// stored row, so the sidebar scrolling under a still mouse stays right.
func (s sidebar) hoveredSession(in sidebarInput) (int, bool) {
	if in.modalOpen {
		return 0, false
	}
	return s.sessionAt(in, s.hoverAt.X, s.hoverAt.Y)
}

// HoverChanges moves the hover on a copy, so the model itself is untouched.
func (m Model) HoverChanges(msg tea.MouseMotionMsg) bool {
	in := m.sidebarInput()
	s := m.sidebar
	before, wasOn := s.hoveredSession(in)
	s.hoverAt = msg.Mouse()
	after, isOn := s.hoveredSession(in)
	return before != after || wasOn != isOn
}

// refreshHover runs after every Update: an open modal drops the position, so
// the highlight stays off until the mouse moves again once it closes.
func (s *sidebar) refreshHover(modalOpen bool) {
	if modalOpen {
		s.hoverAt = tea.Mouse{}
	}
}
