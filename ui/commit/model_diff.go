package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
)

func (m *Model) syncDiffViewport() {
	l := m.layout()
	_, diffH := l.diffSize()
	bodyW := l.diffRenderWidth()
	bodyH := max(0, diffH-2)
	m.diff.diffModel.Reflow(bodyW)
	m.diff.diffModel.SyncViewport(bodyW, bodyH)
	m.diff.ensureActiveVisible()
	m.sidebar.fileTreeModel.SetVisibleHeight(l.filesInnerHeight())
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

func (m *Model) adjustDiffContextLines(delta int) tea.Cmd {
	if m.diff.adjustDiffContextLines(delta) {
		m.refreshDiff()
	}
	return nil
}

func (m *Model) diffActionCtx() diffActionCtx {
	file, ok := m.sidebar.selectedCommitFile()
	return diffActionCtx{
		worktreeRoot: m.worktreeRoot,
		home:         m.settings.Home(),
		terminal:     m.settings.Terminal,
		commentTerm:  ui.DetectTerminal(),
		focusDiff:    m.focusDiff,
		filePath:     file.Path,
		hasFile:      ok,
		diff:         &m.diff,
	}
}
