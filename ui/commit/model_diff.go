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
	m.diffModel.Reflow(bodyW)
	if m.search.HasQuery() {
		m.search.SetMatches(m.computeDiffSearchMatches(m.search.Query()))
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

func (m *Model) ensureActiveVisible() {
	m.diffModel.EnsureActiveVisible(m.diffModel.NavMode())
}

func (m Model) editorLineForCurrentSelection() int {
	if !m.focusDiff {
		return 0
	}
	diff := m.diffModel.DataRef()
	if m.diffModel.NavMode() == diffview.NavModeLine {
		if diff.ActiveLine < 0 || diff.ActiveLine >= len(diff.Parsed.Changed) {
			return 0
		}
		cl := diff.Parsed.Changed[diff.ActiveLine]
		if cl.NewLine > 0 {
			return cl.NewLine
		}
		return cl.OldLine
	}
	if diff.ActiveHunk < 0 || diff.ActiveHunk >= len(diff.Parsed.Hunks) {
		return 0
	}
	h := diff.Parsed.Hunks[diff.ActiveHunk]
	if h.NewStart > 0 {
		return h.NewStart
	}
	return h.OldStart
}
