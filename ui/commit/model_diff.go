package commit

import (
	"strings"

	"github.com/elentok/gx/ui/explorer"
)

func (m *Model) diffPaneSize() (int, int) {
	headerH := maxInt(4, len(m.headerLines())+2)
	contentH := maxInt(5, m.height-1-headerH-1)
	if m.width < 90 {
		filesH := maxInt(5, contentH/3)
		diffH := maxInt(5, contentH-filesH)
		return m.width, diffH
	}
	leftW := maxInt(24, m.width/4)
	if leftW > m.width-40 {
		leftW = m.width - 40
	}
	if leftW < 24 {
		leftW = 24
	}
	return m.width - leftW, contentH
}

func (m *Model) syncDiffViewport() {
	diffW, diffH := m.diffPaneSize()
	bodyW := maxInt(1, diffW-4)
	bodyH := maxInt(0, diffH-2)
	explorer.ReflowSectionData(&m.section, bodyW, m.wrapSoft)
	if strings.TrimSpace(m.searchQuery) != "" {
		cursor := m.searchCursor
		m.recomputeSearchMatches()
		if len(m.searchMatches) > 0 {
			if cursor >= len(m.searchMatches) {
				cursor = len(m.searchMatches) - 1
			}
			if cursor < 0 {
				cursor = 0
			}
			m.searchCursor = cursor
		}
	}
	m.diffViewport.SetWidth(bodyW)
	m.diffViewport.SetHeight(bodyH)
	m.diffViewport.SetContentLines(m.section.ViewLines)
	m.ensureActiveVisible()
}

func (m *Model) activeRawLineIndex() int {
	return explorer.ActiveRawLineIndex(m.section, m.diffNavMode)
}

func (m *Model) ensureActiveVisible() {
	explorer.EnsureActiveVisible(m.section, &m.diffViewport, m.diffNavMode)
}

func (m *Model) moveDiffActive(delta int) {
	if !explorer.MoveActive(&m.section, &m.diffViewport, m.diffNavMode, delta, false) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
	m.ensureActiveVisible()
}

func (m *Model) scrollDiffPage(direction int) {
	explorer.ScrollPage(&m.diffViewport, direction)
}

func (m *Model) jumpDiffTop() {
	if !explorer.JumpTop(&m.section, &m.diffViewport, m.diffNavMode) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) jumpDiffBottom() {
	if !explorer.JumpBottom(&m.section, &m.diffViewport, m.diffNavMode) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) syncSearchCursorFromDiffFocus() {
	if strings.TrimSpace(m.searchQuery) == "" || len(m.searchMatches) == 0 || !m.focusDiff {
		return
	}
	if i := explorer.CurrentDiffSearchMatchIndex(m.section, m.searchMatches, m.diffNavMode); i >= 0 {
		m.searchCursor = i
	}
}
