package status

import (
	"fmt"
	"time"

	"github.com/elentok/gx/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Init() tea.Cmd {
	if m.settings.EnableNavigation {
		return tea.Batch(statusTickCmd(), statusStartupLoadCmd())
	}
	return tea.Batch(statusTickCmd(), m.cmdColorizeDiffsForSelection(), m.cmdLoadBranchSync())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.help.SetWidth(msg.Width)
		var cmd tea.Cmd
		if m.renderMode == renderSideBySide {
			cmd = m.reloadDiffsForSelection()
		}
		m.syncDiffViewports()
		return m, cmd
	case tea.FocusMsg:
		cmd := m.refreshPreserveScroll()
		return m, cmd
	case statusTickMsg:
		if m.statusMsg != "" && !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
			m.clearStatus()
		}
		return m, statusTickCmd()
	case statusStartupLoadMsg:
		m.reloadBranchState()
		colorizeCmd := m.reloadDiffsForSelection()
		return m, tea.Batch(colorizeCmd, m.cmdLoadBranchSync())
	case actionPollMsg:
		var actionCmd tea.Cmd
		if m.runningRunner != nil {
			if chunk := m.runningRunner.Consume(); chunk != "" {
				m.appendRunningOutput(chunk)
			}
			if !m.runningDone {
				if !m.credentialOpen {
					if prompt, ok := m.runningRunner.Prompt(); ok {
						m.openCredentialPrompt(prompt)
					}
				}
				if res, done := m.runningRunner.Result(); done {
					m.runningDone = true
					actionCmd = m.handleActionResult(res)
				}
			}
		}
		if m.runningOpen && !m.runningDone {
			return m, tea.Batch(actionCmd, actionPollCmd())
		}
		return m, actionCmd
	case diffReloadMsg:
		if msg.seq == m.diffReloadSeq && m.focus == focusStatus {
			return m, m.reloadDiffsForSelection()
		}
		return m, nil
	case diffColorizeMsg:
		if msg.seq != m.colorizeSeq || msg.filePath != m.activeFilePath {
			return m, nil
		}
		sideBySide := m.renderMode == renderSideBySide
		if msg.unstagedColor != "" {
			m.unstaged = buildSectionState(msg.unstagedRaw, msg.unstagedColor, m.unstaged, sideBySide)
		}
		if msg.stagedColor != "" {
			m.staged = buildSectionState(msg.stagedRaw, msg.stagedColor, m.staged, sideBySide)
		}
		m.syncDiffViewports()
		return m, nil
	case branchSyncLoadedMsg:
		if msg.branchName == m.branchName {
			m.branchSync = msg.sync
		}
		return m, nil
	case tea.MouseWheelMsg:
		if m.handleMouseWheel(msg) {
			return m, nil
		}
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if m.runningOpen && !m.runningDone && m.runningRunner != nil {
				m.runningRunner.Cancel()
				m.setStatus("cancel requested")
				return m, nil
			}
			return m, tea.Quit
		}
		if msg.String() == "q" && !m.settings.EnableNavigation {
			if m.runningRunner != nil && !m.runningDone {
				m.runningRunner.Cancel()
			}
			return m, tea.Quit
		}
		if m.credentialOpen {
			return m.handleCredentialKey(msg)
		}
		if m.runningOpen {
			return m.handleRunningKey(msg)
		}
		if m.outputOpen {
			return m.handleOutputKey(msg)
		}
		if m.confirmOpen {
			return m.handleConfirmKey(msg)
		}
		if m.errorOpen {
			return m.handleErrorKey(msg)
		}
		if m.helpOpen {
			return m.handleHelpKey(msg)
		}
		if m.searchMode != searchModeNone {
			return m.handleSearchKey(msg)
		}
		if cmd, handled := m.handleSearchNavigateKey(msg); handled {
			return m, cmd
		}
		if msg.String() == "/" {
			m.enterSearchMode()
			return m, nil
		}
		if handledModel, cmd, handled := m.handleChordKey(msg); handled {
			return handledModel, cmd
		}
		if m.focus == focusStatus {
			return m.handleStatusKey(msg)
		}
		return m.handleDiffKey(msg)
	case flashTickMsg:
		if m.flash.active {
			m.flash.frames--
			if m.flash.frames <= 0 {
				m.flash.active = false
				return m, nil
			}
			return m, nextFlashCmd()
		}
	case commitFinishedMsg:
		if msg.err != nil {
			m.showGitError(fmt.Errorf("commit failed: %w", msg.err))
			return m, nil
		}
		if msg.splitApp != "" {
			m.setStatus("opened " + msg.splitApp + " split: git commit")
			return m, nil
		}
		m.setStatus(ui.MessageClosed("git commit"))
		return m, m.refresh()
	case lazygitLogFinishedMsg:
		if msg.err != nil {
			m.setStatus("lazygit log failed: " + msg.err.Error())
			return m, nil
		}
		m.setStatus(ui.MessageClosed("lazygit log"))
		return m, m.refresh()
	case editFileFinishedMsg:
		if msg.err != nil {
			m.setStatus("edit failed: " + msg.err.Error())
			return m, nil
		}
		m.setStatus(ui.MessageClosed("editor"))
		return m, m.refresh()
	}
	return m, nil
}

func (m Model) handleErrorKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.errorOpen = false
		return m, nil
	}
	var cmd tea.Cmd
	m.errorVP, cmd = m.errorVP.Update(msg)
	return m, cmd
}

func (m Model) handleHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc", "enter":
		m.helpOpen = false
		return m, nil
	}
	var cmd tea.Cmd
	m.helpVP, cmd = m.helpVP.Update(msg)
	return m, cmd
}
