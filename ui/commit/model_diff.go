package commit

import tea "charm.land/bubbletea/v2"

func (m *Model) diffPaneSize() (int, int) {
	_, contentH := m.layoutHeights()
	if m.width < 90 {
		_, diffH := m.narrowPaneHeights(contentH)
		return m.width, diffH
	}
	leftW := m.sidebar.filesPaneWidth(m.filesInput(), contentH)
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
	m.diff.diffModel.Reflow(bodyW)
	m.diff.diffModel.SyncViewport(bodyW, bodyH)
	m.diff.ensureActiveVisible()
	m.sidebar.fileTreeModel.SetVisibleHeight(m.filesInnerHeight())
}

func (m *Model) moveToAdjacentFile(delta int) {
	if !m.sidebar.moveToAdjacentFile(delta) {
		return
	}
	m.refreshDiff()
	if m.focusDiff {
		m.diff.ensureActiveVisible()
	}
}

func (m *Model) filesInnerHeight() int {
	_, contentH := m.layoutHeights()
	if m.width < 90 {
		filesH, _ := m.narrowPaneHeights(contentH)
		return max(1, filesH-2)
	}
	return max(1, contentH-2)
}

func (m *Model) adjustDiffContextLines(delta int) tea.Cmd {
	if m.diff.adjustDiffContextLines(delta) {
		m.refreshDiff()
	}
	return nil
}

func (m Model) editorLineForCurrentSelection() int {
	return m.diff.editorLineForCurrentSelection(m.focusDiff)
}
