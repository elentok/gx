package stage

import (
	"fmt"
	"time"

	"gx/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Init() tea.Cmd {
	return statusTickCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.help.SetWidth(msg.Width)
		if m.renderMode == renderSideBySide {
			m.reloadDiffsForSelection()
		}
		m.syncDiffViewports()
		return m, nil
	case tea.FocusMsg:
		m.refreshPreserveScroll()
		return m, nil
	case statusTickMsg:
		if m.statusMsg != "" && !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
			m.clearStatus()
		}
		return m, statusTickCmd()
	case actionPollMsg:
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
					m.handleActionResult(res)
				}
			}
		}
		if m.runningOpen && !m.runningDone {
			return m, actionPollCmd()
		}
		return m, nil
	case diffReloadMsg:
		if msg.seq == m.diffReloadSeq && m.focus == focusStatus {
			m.reloadDiffsForSelection()
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
		if msg.String() == "q" {
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
		m.refresh()
		return m, nil
	case lazygitLogFinishedMsg:
		if msg.err != nil {
			m.setStatus("lazygit log failed: " + msg.err.Error())
			return m, nil
		}
		m.setStatus(ui.MessageClosed("lazygit log"))
		m.refresh()
		return m, nil
	case editFileFinishedMsg:
		if msg.err != nil {
			m.setStatus("edit failed: " + msg.err.Error())
			return m, nil
		}
		m.setStatus(ui.MessageClosed("editor"))
		m.refresh()
		return m, nil
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
