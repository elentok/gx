package status

import tea "charm.land/bubbletea/v2"

func (m *Model) switchDiffSection() {
	m.diffarea.ToggleSection()
	m.syncDiffViewports()
	m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
}

func (m *Model) focusDiffPane() { m.focus = focusDiff }

func (m *Model) diffLoadEnv() diffLoadEnv {
	file, ok := m.selectedStatusFile()
	return diffLoadEnv{
		host:         m,
		diffarea:     &m.diffarea,
		worktreeRoot: m.worktreeRoot,
		home:         m.settings.Home(),
		file:         file,
		hasFile:      ok,
		contextLines: m.currentDiffContextLines(),
		renderWidth:  m.deltaRenderWidth(),
	}
}

func (m *Model) diffActionCtx() diffActionCtx {
	file, ok := m.selectedStatusFile()
	return diffActionCtx{
		worktreeRoot: m.worktreeRoot,
		home:         m.settings.Home(),
		terminal:     m.settings.Terminal,
		focus:        m.focus,
		file:         file,
		hasFile:      ok,
		diffarea:     &m.diffarea,
	}
}

func (m *Model) reloadDiffsForSelection() tea.Cmd {
	return m.diffLoader.reloadDiffsForSelection(m.diffLoadEnv())
}

func (m *Model) applySelection() tea.Cmd {
	return m.diffLoader.applySelection(m.diffLoadEnv())
}

func (m *Model) enterDiffFromStatus(resetSection bool) tea.Cmd {
	return m.diffLoader.enterDiffFromStatus(m.diffLoadEnv(), resetSection)
}
