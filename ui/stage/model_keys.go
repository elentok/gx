package stage

import tea "charm.land/bubbletea/v2"

func (m Model) handleChordKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	key := msg.String()
	shiftG := (msg.Mod&tea.ModShift) != 0 && (msg.Code == 'g' || msg.Code == 'G' || msg.Text == "g" || msg.Text == "G")
	isUpperG := key == "G" || key == "shift+g" || msg.Text == "G" || msg.ShiftedCode == 'G' || shiftG
	isLowerG := key == "g" && !isUpperG && (msg.Mod&tea.ModShift) == 0
	if m.keyPrefix == "c" {
		m.keyPrefix = ""
		if key == "c" {
			m.setStatus("opening git commit...")
			return m, cmdGitCommit(m.worktreeRoot), true
		}
		if key == "esc" {
			m.clearStatus()
			return m, nil, true
		}
	}
	if m.keyPrefix == "g" {
		m.keyPrefix = ""
		if isLowerG {
			m.jumpToTop()
			if m.focus == focusStatus {
				return m, m.scheduleDiffReload(), true
			}
			return m, nil, true
		}
		if isUpperG {
			m.jumpToBottom()
			if m.focus == focusStatus {
				return m, m.scheduleDiffReload(), true
			}
			return m, nil, true
		}
		if key == "esc" {
			m.clearStatus()
			return m, nil, true
		}
	}
	if m.keyPrefix == "y" {
		m.keyPrefix = ""
		switch key {
		case "c":
			m.yankContextForAI()
			return m, nil, true
		case "f":
			m.yankFilename()
			return m, nil, true
		case "esc":
			m.clearStatus()
			return m, nil, true
		}
	}
	if m.keyPrefix == "o" {
		m.keyPrefix = ""
		if key == "l" {
			m.setStatus("opening lazygit log...")
			return m, cmdLazygitLog(m.worktreeRoot), true
		}
		if key == "esc" {
			m.clearStatus()
			return m, nil, true
		}
	}
	if key == "c" {
		m.keyPrefix = "c"
		m.setStatus("cc: git commit")
		return m, nil, true
	}
	if key == "y" {
		m.keyPrefix = "y"
		m.setStatus("yc: yank AI context · yf: yank filename")
		return m, nil, true
	}
	if key == "o" {
		m.keyPrefix = "o"
		m.setStatus("ol: open lazygit log")
		return m, nil, true
	}
	if isLowerG {
		m.keyPrefix = "g"
		m.setStatus("gg: jump to top")
		return m, nil, true
	}
	if isUpperG {
		m.keyPrefix = ""
		m.jumpToBottom()
		if m.focus == focusStatus {
			return m, m.scheduleDiffReload(), true
		}
		return m, nil, true
	}
	m.keyPrefix = ""
	return m, nil, false
}

func (m Model) handleStatusKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.selected < len(m.statusEntries)-1 {
			m.selected++
			m.onStatusSelectionChanged()
			return m, m.scheduleDiffReload()
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
			m.onStatusSelectionChanged()
			return m, m.scheduleDiffReload()
		}
	case "h", "left":
		if m.focusParentInStatus() {
			return m, m.scheduleDiffReload()
		}
		m.collapseSelectedDir()
		m.reloadDiffsForSelection()
	case "l", "right":
		entry, ok := m.selectedStatusEntry()
		if ok && entry.Kind == statusEntryFile {
			m.enterDiffFromStatus(false)
			return m, nil
		}
		m.expandSelectedDir()
		m.reloadDiffsForSelection()
	case "r":
		m.refresh()
	case "p":
		m.startPullAction()
		return m, actionPollCmd()
	case "P":
		m.openCheckingDivergence()
		return m, cmdPushPreflight(m.worktreeRoot)
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
	case "ctrl+d":
		if m.scrollStatusPage(1) {
			return m, m.scheduleDiffReload()
		}
	case "ctrl+u":
		if m.scrollStatusPage(-1) {
			return m, m.scheduleDiffReload()
		}
	case "space", " ":
		m.toggleStageStatusEntry()
		m.reloadDiffsForSelection()
	case "enter":
		if m.toggleDirOnEnter() {
			m.reloadDiffsForSelection()
			return m, nil
		}
		m.enterDiffFromStatus(false)
	case "?":
		m.showHelpOverlay()
	case "d":
		m.openDiscardStatusConfirm()
	}
	return m, nil
}

func (m Model) handleDiffKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
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
		m.syncDiffViewports()
		m.ensureActiveVisible(m.currentSection())
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
		m.openCheckingDivergence()
		return m, cmdPushPreflight(m.worktreeRoot)
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
	}
	return m, nil
}
