package status

import (
	"fmt"
	"time"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/explorer"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Init() tea.Cmd {
	if m.settings.EnableNavigation {
		return tea.Batch(statusTickCmd(), statusStartupLoadCmd())
	}
	return tea.Batch(statusTickCmd(), m.cmdLoadBranchSync())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tea.FocusMsg:
		return m, m.refreshPreserveScroll()
	case statusTickMsg:
		return m.handleStatusTick()
	case statusStartupLoadMsg:
		return m.handleStartupLoad()
	case actionPollMsg:
		return m.handleActionPoll()
	case diffReloadMsg:
		return m.handleDiffReload(msg)
	case branchSyncLoadedMsg:
		return m.handleBranchSyncLoaded(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheelMsg(msg)
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	case flashTickMsg:
		return m.handleFlashTick()
	case commitFinishedMsg:
		return m.handleCommitFinished(msg)
	case lazygitLogFinishedMsg:
		return m.handleLazygitLogFinished(msg)
	case editFileFinishedMsg:
		return m.handleEditFileFinished(msg)
	}
	return m, nil
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.ready = true

	var helpCmd tea.Cmd
	var reloadCmd tea.Cmd
	if m.renderMode == renderSideBySide {
		reloadCmd = m.reloadDiffsForSelection()
	}
	m.syncDiffViewports()
	m.help, helpCmd = m.help.Update(msg)
	return m, tea.Batch(reloadCmd, helpCmd)
}

func (m Model) handleStatusTick() (tea.Model, tea.Cmd) {
	if m.statusMsg != "" && !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
		m.clearStatus()
	}
	return m, statusTickCmd()
}

func (m Model) handleStartupLoad() (tea.Model, tea.Cmd) {
	m.reloadBranchState()
	reloadCmd := m.reloadDiffsForSelection()
	return m, tea.Batch(reloadCmd, m.cmdLoadBranchSync())
}

func (m Model) handleActionPoll() (tea.Model, tea.Cmd) {
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
}

func (m Model) handleDiffReload(msg diffReloadMsg) (tea.Model, tea.Cmd) {
	if msg.seq == m.diffReloadSeq && m.focus == focusStatus {
		return m, m.reloadDiffsForSelection()
	}
	return m, nil
}

func (m Model) handleBranchSyncLoaded(msg branchSyncLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.branchName == m.branchName {
		m.branchSync = msg.sync
	}
	return m, nil
}

func (m Model) handleMouseWheelMsg(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.handleMouseWheel(msg) {
		return m, nil
	}
	return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	if msg.String() == "?" {
		m.help.Open(m.width, m.height)
		return m, nil
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
	if m.help.IsOpen {
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		return m, cmd
	}
	if m.searchMode != explorer.SearchModeNone {
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
}

func (m Model) handleFlashTick() (tea.Model, tea.Cmd) {
	if m.flash.active {
		m.flash.frames--
		if m.flash.frames <= 0 {
			m.flash.active = false
			return m, nil
		}
		return m, nextFlashCmd()
	}
	return m, nil
}

func (m Model) handleCommitFinished(msg commitFinishedMsg) (tea.Model, tea.Cmd) {
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
}

func (m Model) handleLazygitLogFinished(msg lazygitLogFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setStatus("lazygit log failed: " + msg.err.Error())
		return m, nil
	}
	m.setStatus(ui.MessageClosed("lazygit log"))
	return m, m.refresh()
}

func (m Model) handleEditFileFinished(msg editFileFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setStatus("edit failed: " + msg.err.Error())
		return m, nil
	}
	m.setStatus(ui.MessageClosed("editor"))
	return m, m.refresh()
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
