package commit

import "github.com/elentok/gx/ui/list"

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
	m.diffModel.Reflow(bodyW)
	if m.search.HasQuery() && m.searchScope == searchScopeDiff {
		matches := m.computeSearchMatches(m.search.Query())
		m.search.SetMatches(matches)
	}
	m.diffModel.SyncViewport(bodyW, bodyH)
	m.ensureActiveVisible()
	m.fileTreeModel.SetVisibleHeight(m.filesInnerHeight())
}

func (m *Model) filesInnerHeight() int {
	headerH := m.headerViewportRowsCount() + 2
	contentH := max(5, m.height-1-headerH-1)
	if m.width < 90 {
		return max(1, max(5, contentH/3)-2)
	}
	return max(1, contentH-2)
}

func (m *Model) moveDiffActive(delta int) {
	if !m.diffModel.MoveActive(delta, false) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
	m.ensureActiveVisible()
}

func (m *Model) ensureActiveVisible() {
	m.diffModel.EnsureActiveVisible(m.diffModel.NavMode())
}

func (m *Model) scrollDiffPage(direction int) {
	m.diffModel.ScrollPage(direction * list.DefaultScroll)
}

func (m *Model) jumpDiffTop() {
	if !m.diffModel.JumpTop() {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) jumpDiffBottom() {
	if !m.diffModel.JumpBottom() {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}
