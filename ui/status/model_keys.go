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
			m.clearStatus()
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
				m.clearStatus()
				return m, nav.Replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.worktreeRoot}), true
			}
			return m, nil, true
		case "s":
			if m.settings.EnableNavigation {
				m.clearStatus()
				return m, nav.Replace(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: m.worktreeRoot}), true
			}
			return m, nil, true
		case "w":
			if m.settings.EnableNavigation {
				m.clearStatus()
				return m, nav.Replace(nav.Route{Kind: nav.RouteWorktrees}), true
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
