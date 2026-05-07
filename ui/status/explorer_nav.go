package status

import "github.com/elentok/gx/ui/explorer"

func (m *Model) moveActive(delta int) {
	sec := m.currentSection()
	if !explorer.MoveActive(&sec.data, &sec.viewport, m.navMode, delta, true) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
	m.ensureActiveVisible(sec)
}

func (m *Model) scrollDiffPage(direction int) {
	sec := m.currentSection()
	explorer.ScrollPage(&sec.viewport, direction)
}

func (m *Model) jumpDiffTop() {
	sec := m.currentSection()
	if !explorer.JumpTop(&sec.data, &sec.viewport, m.navMode) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) jumpDiffBottom() {
	sec := m.currentSection()
	if !explorer.JumpBottom(&sec.data, &sec.viewport, m.navMode) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}
