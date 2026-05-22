package status

import (
	"fmt"
	"time"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Init() tea.Cmd {
	if m.settings.EnableNavigation {
		return tea.Batch(renderTickCmd(), statusStartupLoadCmd())
	}
	return tea.Batch(renderTickCmd(), m.cmdLoadBranchSync())
}

func renderTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return renderTickMsg{} })
}

func (m Model) Update(msg tea.Msg) (next tea.Model, cmd tea.Cmd) {
	prevRoute, prevOK := m.currentRouteIdentity()
	defer func() {
		nextModel, ok := next.(Model)
		if !ok {
			return
		}
		route, routeOK := nextModel.currentRouteIdentity()
		cmd = nav.AppendRouteChanged(cmd, m.settings.EnableNavigation, prevRoute, prevOK, route, routeOK)
	}()

	if m.bump.IsOpen {
		return m.handleBumpUpdate(msg)
	}
	if m.push.IsOpen {
		return m.handlePushUpdate(msg)
	}
	if m.pull.IsOpen {
		return m.handlePullUpdate(msg)
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tea.FocusMsg:
		return m, m.refreshPreserveScroll()
	case renderTickMsg:
		return m, renderTickCmd()
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
	case editCommentFinishedMsg:
		return m.handleEditCommentFinished(msg)
	case search.JumpToMatchMsg:
		return m.handleJumpToMatch(msg)
	case search.SearchQueryUpdatedMsg:
		return m.handleSearchQueryUpdated(msg)
	}
	return m, nil
}

func (m Model) handleFileTreeRebuildRequested() (Model, tea.Cmd) {
	m.statusData.statusEntries, m.statusData.statusRows = buildStatusEntriesAndRows(m.statusData.files, m.fileTreeModel.CollapsedDirs())
	m.reconcileFileTreeFromStatusState()
	return m, m.reloadDiffsForSelection()
}

func (m Model) handleFileTreeOpenSelected() (Model, tea.Cmd) {
	entry, ok := m.selectedFiletreeEntry()
	if ok && entry.Kind == statusEntryFile {
		return m, m.enterDiffFromStatus(false)
	}
	return m, nil
}

func (m Model) handleSearchQueryUpdated(msg search.SearchQueryUpdatedMsg) (Model, tea.Cmd) {
	if m.focus == focusFiletree {
		matches := m.computeSearchMatches(msg.Query)
		return m, m.fileTreeModel.Search().SetMatchesAndJump(matches)
	}
	matches := m.computeSearchMatches(msg.Query)
	return m, m.currentDiffSearch().SetMatchesAndJump(matches)
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.ready = true

	var helpCmd tea.Cmd
	var reloadCmd tea.Cmd
	if m.diffarea.RenderMode() == diffview.RenderModeSideBySide {
		reloadCmd = m.reloadDiffsForSelection()
	}
	m.syncDiffViewports()
	m.help, helpCmd = m.help.Update(msg)
	return m, tea.Batch(reloadCmd, helpCmd)
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
	if msg.seq == m.diffReloadSeq && m.focus == focusFiletree {
		return m, m.reloadDiffsForSelection()
	}
	return m, nil
}

func (m Model) handleBranchSyncLoaded(msg branchSyncLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.branchName == m.statusData.branchName {
		m.statusData.branchSync = msg.sync
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
			return m, notify.Info("cancel requested")
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
		m.keys.Reset()
		return m, nil
	}
	if m.credentialOpen {
		return m.handleCredentialKey(msg)
	}
	if m.runningOpen {
		return m.handleRunningKey(msg)
	}
	if m.output.IsOpen {
		next, cmd := m.output.Update(msg)
		m.output = next
		return m, cmd
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

	// In search input mode the child model owns all keystrokes; bypass Manager.
	if m.InputFocused() {
		return m.delegateToChild(msg)
	}

	match, consumed := m.keys.Process(msg)
	if match != nil {
		return m.dispatchBinding(match.ID, msg)
	}
	if consumed {
		return m, nil
	}

	return m.delegateToChild(msg)
}

func (m Model) handleFlashTick() (tea.Model, tea.Cmd) {
	if m.diffarea.Flash.Active {
		m.diffarea.Flash.Frames--
		if m.diffarea.Flash.Frames <= 0 {
			m.diffarea.Flash.Active = false
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
		return m, notify.Info("opened " + msg.splitApp + " split: git commit")
	}
	return m, tea.Batch(notify.Info(ui.MessageClosed("git commit")), m.refresh())
}

func (m Model) handleLazygitLogFinished(msg lazygitLogFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("lazygit log failed: " + msg.err.Error())
	}
	return m, tea.Batch(notify.Info(ui.MessageClosed("lazygit log")), m.refresh())
}

func (m Model) handleEditFileFinished(msg editFileFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("edit failed: " + msg.err.Error())
	}
	return m, tea.Batch(notify.Info(ui.MessageClosed("editor")), m.refresh())
}

func (m Model) handleEditCommentFinished(msg editCommentFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("comment edit failed: " + msg.err.Error())
	}
	if msg.splitApp != "" {
		return m, notify.Info("opened " + msg.splitApp + " split: comment editor")
	}
	return m, tea.Batch(notify.Info(ui.MessageClosed("comment editor")), m.refresh())
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
