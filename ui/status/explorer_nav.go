package status

import "github.com/elentok/gx/ui/explorer"

func (m *Model) moveActive(delta int) {
	sec := m.currentSection()
	data := toExplorerSectionData(*sec)
	if !explorer.MoveActive(&data, &sec.viewport, m.navMode, delta, true) {
		return
	}
	*sec = fromExplorerSectionData(data, sec.viewport)
	m.syncSearchCursorFromDiffFocus()
	m.ensureActiveVisible(sec)
}

func (m *Model) scrollDiffPage(direction int) {
	sec := m.currentSection()
	explorer.ScrollPage(&sec.viewport, direction)
}

func (m *Model) jumpDiffTop() {
	sec := m.currentSection()
	data := toExplorerSectionData(*sec)
	if !explorer.JumpTop(&data, &sec.viewport, m.navMode) {
		return
	}
	*sec = fromExplorerSectionData(data, sec.viewport)
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) jumpDiffBottom() {
	sec := m.currentSection()
	data := toExplorerSectionData(*sec)
	if !explorer.JumpBottom(&data, &sec.viewport, m.navMode) {
		return
	}
	*sec = fromExplorerSectionData(data, sec.viewport)
	m.syncSearchCursorFromDiffFocus()
}
