package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
)

// FilterMotion is the program's tea.WithFilter. Bubbletea runs View after
// every Update, and an all-motion page gets an event per mouse cell, so it
// drops each motion event that would not move the active page's hover
// highlight. Pages that are not a ui.HoverPage get every event as before.
func FilterMotion(model tea.Model, msg tea.Msg) tea.Msg {
	motion, ok := msg.(tea.MouseMotionMsg)
	if !ok {
		return msg
	}
	m, ok := model.(Model)
	if !ok {
		return msg
	}
	page, ok := m.activePage().model.(ui.HoverPage)
	if !ok {
		return msg
	}
	// Neither overlay reacts to motion, and it hides the page.
	if m.switcher.isOpen || m.notifyHistory.IsOpen {
		return nil
	}
	motion.Y -= ui.ChromeRows
	if !page.HoverChanges(motion) {
		return nil
	}
	return msg
}
