package status

import "github.com/elentok/gx/ui/diffview"

func (m *Model) moveActive(delta int) {
	sec := m.currentSection()
	if !diffview.MoveActive(&sec.data, &sec.viewport, m.navMode, delta, true) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
	m.ensureActiveVisible(sec)
}

func (m *Model) scrollDiffPage(direction int) {
	sec := m.currentSection()
	diffview.ScrollPage(&sec.viewport, direction)
}

func (m *Model) jumpDiffTop() {
	sec := m.currentSection()
	if !diffview.JumpTop(&sec.data, &sec.viewport, m.navMode) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) jumpDiffBottom() {
	sec := m.currentSection()
	if !diffview.JumpBottom(&sec.data, &sec.viewport, m.navMode) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}
