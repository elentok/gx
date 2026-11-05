package status

import (
	"github.com/elentok/gx/ui/status/diffarea"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) switchDiffSection() {
	m.diffarea.ToggleSection()
	m.syncDiffViewports()
	m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
}

func (m *Model) focusDiffPane() { m.focus = focusDiff }

func (m *Model) diffLoadEnv() diffLoadEnv {
	file, ok := m.selectedStatusFile()
	_, diffW := m.splitWidth()
	if m.diffarea.Fullscreen && m.focus == focusDiff {
		diffW = m.width
	}
	return diffLoadEnv{
		host:         m,
		diffarea:     &m.diffarea,
		worktreeRoot: m.worktreeRoot,
		home:         m.settings.Home(),
		file:         file,
		hasFile:      ok,
		contextLines: m.diffarea.ContextLines(),
		renderWidth:  diffarea.RenderWidth(diffW, m.settings.UseNerdFontIcons),
	}
}

func (m *Model) adjustDiffContextLines(delta int) tea.Cmd {
	if !m.diffarea.AdjustContextLines(delta) {
		return nil
	}
	cmd := m.reloadDiffsForSelection()
	m.syncDiffViewports()
	if m.focus == focusDiff {
		m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
	}
	return cmd
}

func (m *Model) toggleRenderMode() tea.Cmd {
	if !m.diffarea.ToggleRenderMode() {
		return nil
	}
	cmd := m.reloadDiffsForSelection()
	m.syncDiffViewports()
	m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
	return cmd
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
