package status

import tea "charm.land/bubbletea/v2"

func (m Model) handleDiffKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "[":
		m.adjustDiffContextLines(-1)
		return m, nil
	case "]":
		m.adjustDiffContextLines(1)
		return m, nil
	case "esc", "q":
		sec := m.currentSection()
		if sec.visualActive {
			sec.visualActive = false
			sec.visualAnchor = sec.activeLine
			return m, nil
		}
		m.focus = focusStatus
		return m, nil
	case "h", "left":
		m.focus = focusStatus
		return m, nil
	case "tab":
		if m.canSwitchSections() {
			if m.section == sectionUnstaged {
				m.section = sectionStaged
			} else {
				m.section = sectionUnstaged
			}
			m.syncDiffViewports()
			m.ensureActiveVisible(m.currentSection())
		}
	case "a":
		sec := m.currentSection()
		sec.visualActive = false
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
		if len(sec.parsed.Changed) == 0 {
			return m, nil
		}
		if !sec.visualActive {
			sec.visualActive = true
			sec.visualAnchor = sec.activeLine
		} else {
			sec.visualActive = false
			sec.visualAnchor = sec.activeLine
		}
		m.ensureActiveVisible(sec)
	case "f":
		m.diffFullscreen = !m.diffFullscreen
		if m.renderMode == renderSideBySide {
			m.reloadDiffsForSelection()
		}
		m.syncDiffViewports()
		m.ensureActiveVisible(m.currentSection())
	case "s":
		m.toggleRenderMode()
	case "w":
		m.wrapSoft = !m.wrapSoft
		m.syncDiffViewports()
		m.ensureActiveVisible(m.currentSection())
	case "r":
		m.refresh()
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
	case "?":
		m.showHelpOverlay()
	case ".":
		if m.moveToAdjacentFile(1) {
			return m, nil
		}
	case ",":
		if m.moveToAdjacentFile(-1) {
			return m, nil
		}
	case "e":
		return m, m.cmdEditSelectedFile()
	}
	return m, nil
}
