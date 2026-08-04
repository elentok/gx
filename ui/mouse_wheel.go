package ui

import tea "charm.land/bubbletea/v2"

// WheelScrollLines is the number of lines a single wheel tick scrolls,
// shared by every mouse-wheel handler so a speed change only happens here.
const WheelScrollLines = 3

// WheelDirection maps a mouse-wheel message's button to a scroll direction:
// -1 for wheel-up, 1 for wheel-down. ok is false for any other button, so
// callers can early-return without scrolling.
func WheelDirection(msg tea.MouseWheelMsg) (dir int, ok bool) {
	switch msg.Mouse().Button {
	case tea.MouseWheelDown:
		return 1, true
	case tea.MouseWheelUp:
		return -1, true
	default:
		return 0, false
	}
}

// ClampScrollOffset bounds offset to [0, total-viewportH], the range in
// which a viewportH-line window into total lines of content stays fully
// populated.
func ClampScrollOffset(offset, total, viewportH int) int {
	maxOffset := max(0, total-viewportH)
	return max(0, min(offset, maxOffset))
}
