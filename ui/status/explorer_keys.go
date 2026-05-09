package status

import tea "charm.land/bubbletea/v2"

func (m Model) handleDiffKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.Code == tea.KeyTab {
		m.cycleFrameForward()
		return m, nil
	}
	switch msg.String() {
	case "[":
		return m, m.adjustDiffContextLines(-1)
	case "]":
		return m, m.adjustDiffContextLines(1)
	case "esc", "q":
		sec := m.currentSection()
		if sec.data.VisualActive {
			sec.data.VisualActive = false
			sec.data.VisualAnchor = sec.data.ActiveLine
			return m, nil
		}
		m.focus = focusStatus
		return m, nil
	case "h", "left":
		m.focus = focusStatus
		return m, nil
	case "a":
		sec := m.currentSection()
		sec.data.VisualActive = false
		if m.navMode == navHunk {
			m.navMode = navLine
		} else {
			m.navMode = navHunk
		}
		m.ensureActiveVisible(m.currentSection())
	case "v":
		sec := m.currentSection()
		if m.navMode == navHunk {
			m.navMode = navLine
		}
		if len(sec.data.Parsed.Changed) == 0 {
			return m, nil
		}
		if !sec.data.VisualActive {
			sec.data.VisualActive = true
			sec.data.VisualAnchor = sec.data.ActiveLine
		} else {
			sec.data.VisualActive = false
			sec.data.VisualAnchor = sec.data.ActiveLine
		}
		m.ensureActiveVisible(sec)
	case "f":
		m.diffFullscreen = !m.diffFullscreen
		var cmd tea.Cmd
		if m.renderMode == renderSideBySide {
			cmd = m.reloadDiffsForSelection()
		}
		m.syncDiffViewports()
		m.ensureActiveVisible(m.currentSection())
		return m, cmd
	case "s":
		return m, m.toggleRenderMode()
	case "w":
		m.wrapSoft = !m.wrapSoft
		m.syncDiffViewports()
		m.ensureActiveVisible(m.currentSection())
	case "r":
		return m, m.refresh()
	case "p":
		m.startPullAction()
		return m, actionPollCmd()
	case "P":
		if err := m.preparePushConfirm(); err != nil {
			m.showGitError(err)
			return m, nil
		}
		return m, nil
	case "b":
		if err := m.prepareRebaseConfirm(); err != nil {
			m.showGitError(err)
			return m, nil
		}
		return m, nil
	case "A":
		if err := m.openAmendConfirm(); err != nil {
			m.showGitError(err)
		}
	case "j", "down":
		m.moveActive(1)
	case "k", "up":
		m.moveActive(-1)
	case "J":
		sec := m.currentSection()
		sec.viewport.ScrollDown(3)
	case "K":
		sec := m.currentSection()
		sec.viewport.ScrollUp(3)
	case "ctrl+d":
		m.scrollDiffPage(1)
	case "ctrl+u":
		m.scrollDiffPage(-1)
	case "space", " ":
		cmd := m.applySelection()
		return m, cmd
	case "d":
		if m.section == sectionStaged {
			cmd := m.applySelection()
			return m, cmd
		}
		m.openDiscardDiffConfirm()
		return m, nil
	case ".":
		if ok, cmd := m.moveToAdjacentFile(1); ok {
			return m, cmd
		}
	case ",":
		if ok, cmd := m.moveToAdjacentFile(-1); ok {
			return m, cmd
		}
	case "e":
		return m, m.cmdEditSelectedFile()
	}
	return m, nil
}
