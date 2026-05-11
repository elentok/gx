package commit

import (
	"github.com/elentok/gx/ui/diffview"
)

func (m *Model) diffPaneSize() (int, int) {
	headerH := m.headerViewportRowsCount() + 2
	contentH := max(5, m.height-1-headerH-1)
	if m.width < 90 {
		filesH := max(5, contentH/3)
		diffH := max(5, contentH-filesH)
		return m.width, diffH
	}
	leftW := m.filesPaneWidth(contentH)
	return m.width - leftW, contentH
}

func (m *Model) currentDiffRenderWidth() int {
	diffW, _ := m.diffPaneSize()
	return max(1, diffW-4)
}

func (m *Model) syncDiffViewport() {
	_, diffH := m.diffPaneSize()
	bodyW := m.currentDiffRenderWidth()
	bodyH := max(0, diffH-2)
	diffview.ReflowDiffBuffer(&m.section, bodyW, m.wrapSoft)
	if m.search.HasQuery() && m.searchScope == searchScopeDiff {
		matches := m.computeSearchMatches(m.search.Query())
		m.search.SetMatches(matches)
	}
	m.diffViewport.SetWidth(bodyW)
	m.diffViewport.SetHeight(bodyH)
	m.diffViewport.SetContentLines(m.section.ViewLines)
	m.ensureActiveVisible()
}

func (m *Model) activeRawLineIndex() int {
	return diffview.ActiveRawLineIndex(m.section, m.diffNavMode)
}

func (m *Model) ensureActiveVisible() {
	diffview.EnsureActiveVisible(m.section, &m.diffViewport, m.diffNavMode)
}

func (m *Model) moveDiffActive(delta int) {
	if !diffview.MoveActive(&m.section, &m.diffViewport, m.diffNavMode, delta, false) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
	m.ensureActiveVisible()
}

func (m *Model) scrollDiffPage(direction int) {
	diffview.ScrollPage(&m.diffViewport, direction)
}

func (m *Model) jumpDiffTop() {
	if !diffview.JumpTop(&m.section, &m.diffViewport, m.diffNavMode) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) jumpDiffBottom() {
	if !diffview.JumpBottom(&m.section, &m.diffViewport, m.diffNavMode) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}

