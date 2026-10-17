package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
)

// newHoverView is ui.NewMainView in all-motion mode, so the terminal reports
// motion with no button held.
func newHoverView(content string) tea.View {
	v := ui.NewMainView(content)
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

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

// queueHoverTarget is QueueModel's hoverTarget: the run-state banner above
// the tree holds no rows.
func (m QueueModel) queueHoverTarget(msg tea.MouseMotionMsg) (int, bool) {
	mouse := msg.Mouse()
	rect := m.queueTreeRect()
	if m.ModalOpen() || !rect.Contains(mouse.X, mouse.Y) {
		return -1, false
	}
	return m.queueTree.EntryAtBodyLine(mouse.Y - rect.Y - 1 - queueHeaderReservedLines)
}

func (m QueueModel) HoverChanges(msg tea.MouseMotionMsg) bool {
	return m.queueTree.SetHoveredIndex(m.queueHoverTarget(msg))
}

func (m QueueModel) handleQueueMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	m.queueTree.SetHoveredIndex(m.queueHoverTarget(msg))
	return m, nil
}

func (m *QueueModel) refreshQueueHover(msg tea.Msg) {
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		m.queueTree.SetHoveredIndex(m.queueHoverTarget(tea.MouseMotionMsg(wheel)))
	} else if m.ModalOpen() {
		m.queueTree.SetHoveredIndex(0, false)
	}
}
