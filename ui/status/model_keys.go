package status

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
)

import tea "charm.land/bubbletea/v2"

func (m Model) handleChordKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	key := msg.String()
	shiftG := (msg.Mod&tea.ModShift) != 0 && (msg.Code == 'g' || msg.Code == 'G' || msg.Text == "g" || msg.Text == "G")
	isUpperG := key == "G" || key == "shift+g" || msg.Text == "G" || msg.ShiftedCode == 'G' || shiftG
	if m.keyPrefix == "c" {
		m.keyPrefix = ""
		if key == "c" {
			m.setStatus(ui.MessageOpening("git commit"))
			return m, cmdGitCommit(m.worktreeRoot, m.settings.Terminal), true
		}
		if key == "esc" {
			m.clearStatus()
			return m, nil, true
		}
	}
	if m.keyPrefix == "y" {
		m.keyPrefix = ""
		switch key {
		case "l":
			m.yankLocationOnly()
			return m, nil, true
		case "a":
			m.yankAllContext()
			return m, nil, true
		case "f":
			m.yankFilename()
			return m, nil, true
		case "y":
			m.yankContentOnly()
			return m, nil, true
		case "esc":
			m.clearStatus()
			return m, nil, true
		}
	}
	if key == "c" {
		m.keyPrefix = "c"
		m.setStatus(m.inlineHints(stageKeyCommit))
		return m, nil, true
	}
	if key == "y" {
		m.keyPrefix = "y"
		m.setStatus(m.inlineHints(stageKeyYankText, stageKeyYankPath, stageKeyYankAll, stageKeyYankName))
		return m, nil, true
	}
	if m.keyPrefix == "g" {
		m.keyPrefix = ""
		switch key {
		case "g":
			m.jumpToTop()
			if m.focus == focusStatus {
				return m, m.scheduleDiffReload(), true
			}
			return m, nil, true
		case "o":
			if m.outputContent == "" {
				m.setStatus(ui.MessageNoOutput())
				return m, nil, true
			}
			m.openOutputModal()
			return m, nil, true
		case "l":
			if m.settings.EnableNavigation {
				return m, nav.Push(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.worktreeRoot}), true
			}
			return m, nil, true
		case "s":
			if m.settings.EnableNavigation {
				return m, nav.Push(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: m.worktreeRoot}), true
			}
			return m, nil, true
		case "w":
			if m.settings.EnableNavigation {
				return m, nav.Push(nav.Route{Kind: nav.RouteWorktrees}), true
			}
			return m, nil, true
		case "esc":
			m.clearStatus()
			return m, nil, true
		default:
			m.clearStatus()
			return m, nil, true
		}
	}
	if key == "g" && !isUpperG {
		m.keyPrefix = "g"
		m.setStatus(m.inlineHints(stageKeyTop, stageKeyOutput, stageKeyGoWorktree, stageKeyGoLog, stageKeyGoStatus))
		return m, nil, true
	}
	if key == "L" {
		m.setStatus(ui.MessageOpening("lazygit log"))
		return m, cmdLazygitLog(m.worktreeRoot), true
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
	case "q":
		if m.settings.EnableNavigation {
			return m, nav.Back()
		}
		return m, tea.Quit
	case "esc":
		if m.settings.EnableNavigation {
			return m, nav.Back()
		}
		return m, nil
	case "[":
		m.adjustDiffContextLines(-1)
		return m, nil
	case "]":
		m.adjustDiffContextLines(1)
		return m, nil
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
	case "s":
		m.toggleRenderMode()
	case "p":
		return m.startPullAction()
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
	case "e":
		return m, m.cmdEditSelectedFile()
	}
	return m, nil
}

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
