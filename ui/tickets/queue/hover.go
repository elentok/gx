package queue

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/tickets/queue/header"
)

// queueHoverTarget is QueueModel's hoverTarget: the run-state banner above
// the tree holds no rows.
func (m QueueModel) queueHoverTarget(msg tea.MouseMotionMsg) (int, bool) {
	mouse := msg.Mouse()
	rect := m.layout().treeRect()
	if m.ModalOpen() || !rect.Contains(mouse.X, mouse.Y) {
		return -1, false
	}
	return m.rows.Tree.EntryAtBodyLine(mouse.Y - rect.Y - 1 - header.HeaderReservedLines)
}

func (m QueueModel) HoverChanges(msg tea.MouseMotionMsg) bool {
	return m.rows.Tree.SetHoveredIndex(m.queueHoverTarget(msg))
}
