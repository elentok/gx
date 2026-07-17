package worktrees

import (
	"fmt"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func (m Model) Init() tea.Cmd {
	return cmdLoadWorktrees(m.repo)
}

func (m Model) InputFocused() bool {
	return m.mode == modeRename || m.mode == modeClone || m.mode == modeNew ||
		m.mode == modeNewAndOpen || m.mode == modeCredentialPrompt || m.mode == modeSearch ||
		m.pull.InputFocused() || m.helpModel.InputFocused()
}

func (m Model) ModalOpen() bool {
	return m.pull.IsOpen || m.confirm.IsOpen
}

func (m Model) Update(msg tea.Msg) (next tea.Model, cmd tea.Cmd) {
	if m.pull.IsOpen {
		return m.handlePullUpdate(msg)
	}

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case gotoPRMsg:
		return m.handleGotoPR(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m = m.resized()
		return m, nil

	case tea.PasteMsg:
		switch m.mode {
		case modeCredentialPrompt, modeRename, modeClone, modeNew, modeNewAndOpen:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		case modeSearch:
			m.search.Start(msg.Content)
			return m.updateSearchMatches()
		}

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.confirm.IsOpen {
			return m.handleConfirmUpdate(msg)
		}
		switch m.mode {
		case modeError:
			return m.handleErrorKey(msg)
		case modeHelp:
			return m.handleHelpKey(msg)
		case modeCredentialPrompt:
			return m.handleCredentialKey(msg)
		case modeRename:
			return m.handleRenameKey(msg)
		case modeClone:
			return m.handleCloneKey(msg)
		case modeNew, modeNewAndOpen:
			return m.handleNewKey(msg)
		case modeTerminalMenu:
			return m.handleTerminalMenuKey(msg)
		case modeYank:
			return m.handleYankKey(msg)
		case modePaste:
			return m.handlePasteModeKey(msg)
		case modeSearch:
			return m.handleSearchKey(msg)
		case modeLogs:
			return m.handleLogsKey(msg)
		case modePushDiverged:
			return m.handlePushDivergedKey(msg)
		case modeDeleteProgress:
			return m, nil
		}
		match, consumed := m.keyManager.Process(msg)
		if match != nil {
			return m.dispatchBinding(match.ID)
		}
		if consumed {
			return m, nil
		}

	case startBatchDeleteMsg:
		return m.enterDeleteProgress(msg.worktrees)

	case deleteResultMsg:
		if m.mode == modeDeleteProgress {
			return m.handleDeleteProgressResult(msg)
		}
		m.spinnerActive = false
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		return m, tea.Batch(notify.Info(fmt.Sprintf("deleted worktree %s", msg.name)), cmdLoadWorktrees(m.repo))

	case renameResultMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		return m, cmdLoadWorktrees(m.repo)

	case cloneResultMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		return m, cmdLoadWorktrees(m.repo)

	case newResultMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		return m, cmdLoadWorktrees(m.repo)

	case newOpenResultMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m = m.enterTerminalMenuFor(msg.name, msg.path)
		return m, cmdLoadWorktrees(m.repo)

	case terminalResultMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		return m, nil

	case yankDataMsg:
		if m.mode != modeYank || msg.worktreePath != m.yankSource.Path {
			return m, nil
		}
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.yankLoading = false
		m.yankChecklist = components.NewChecklist(changesToChecklistItems(msg.changes))
		return m, nil

	case promptableJobStartMsg:
		args := promptableJobArgs(m.repo, msg.kind, msg.wt)
		runner := components.NewCommandRunnerWithPolicy(msg.wt.Path, "git", components.CredentialPolicyPrompt, args...)
		runner.Start()
		log := ui.CommandOutputLogFrom(msg.initialLog)
		w := msg.wt
		m.jobRunner = runner
		m.jobKind = msg.kind
		m.jobWorktree = &w
		m.jobLog = log
		m.jobStashed = msg.stashed
		m.spinnerActive = true
		m.spinnerLabel = promptableJobLabel(msg.kind, msg.wt)
		return m, m.spinner.Tick

	case spinner.TickMsg:
		if m.jobRunner != nil {
			if prompt, ok := m.jobRunner.Prompt(); ok && m.mode != modeCredentialPrompt {
				m = m.enterCredentialPrompt(prompt)
			}
			if err, done := m.jobRunner.Result(); done {
				return m.finishPromptableJob(err)
			}
		}
		if m.spinnerActive || m.previewLoading || m.mode == modeDeleteProgress {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			if m.previewLoading {
				m.viewport.SetContent(m.previewContent())
			}
			return m, cmd
		}
		return m, nil

	case rebasePreflightMsg:
		return m.enterConfirmWithCancel(
			fmt.Sprintf("Stash changes before rebasing %s?", msg.wt.Branch),
			cmdRebase(msg.repo, msg.wt, true),
			"Rebasing "+msg.wt.Name+"…",
			"rebase aborted (dirty worktree)",
		), nil

	case rebaseResultMsg:
		m.spinnerActive = false
		m.lastJobLog = msg.log
		m.lastJobLabel = "Rebase output"
		if msg.err != nil {
			if msg.stashed {
				prompt := fmt.Sprintf("Rebase failed: %s\n\nPop stash?", msg.err.Error())
				m = m.enterConfirmDefaultYes(prompt, cmdStashPop(msg.wtPath, "rebase", msg.log), "Popping stash…")
				return m, nil
			}
			return m.showError(msg.err.Error()), nil
		}
		if msg.stashed {
			m.spinnerActive = true
			m.spinnerLabel = "Popping stash…"
			return m, tea.Batch(cmdStashPop(msg.wtPath, "rebase", msg.log), m.spinner.Tick)
		}
		cmds = append(cmds, notify.Info(ui.MessageComplete("rebase")))
		for _, w := range m.worktrees {
			if w.Branch != "" {
				cmds = append(cmds, cmdLoadBaseStatus(m.repo, w.Branch))
			}
		}
		if wt := m.cursorWorktree(); wt != nil && wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadPreviewData(m.repo, *wt))
		}
		return m, tea.Batch(cmds...)

	case stashPopResultMsg:
		m.spinnerActive = false
		m.lastJobLog = msg.log
		if msg.err != nil {
			return m.showError(fmt.Sprintf("Stash pop failed: %s", msg.err.Error())), nil
		}
		var stashPopMsg string
		switch msg.opLabel {
		case "pull":
			stashPopMsg = "pull complete (stash restored)"
		case "rebase":
			stashPopMsg = "rebase complete (stash restored)"
		default:
			stashPopMsg = "stash restored"
		}
		cmds = append(cmds, notify.Info(stashPopMsg))
		if wt := m.cursorWorktree(); wt != nil {
			cmds = append(cmds, cmdLoadDirtyStatus(*wt), cmdLoadPreviewData(m.repo, *wt))
			if wt.Branch != "" {
				cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch))
			}
		}
		switch msg.opLabel {
		case "rebase":
			for _, w := range m.worktrees {
				if w.Branch != "" {
					cmds = append(cmds, cmdLoadBaseStatus(m.repo, w.Branch))
				}
			}
		case "pull":
			if wt := m.cursorWorktree(); wt != nil && wt.Branch == m.repo.MainBranch {
				for _, w := range m.worktrees {
					if w.Branch != "" {
						cmds = append(cmds, cmdLoadBaseStatus(m.repo, w.Branch))
					}
				}
			}
		}
		return m, tea.Batch(cmds...)

	case pushResultMsg:
		m.spinnerActive = false
		m.lastJobLog = msg.log
		m.lastJobLabel = "Push output"
		if msg.divergence != nil {
			wt := m.cursorWorktree()
			if wt != nil {
				return m.enterPushDivergedMode(*wt, msg.divergence), nil
			}
			return m.showError("cannot resolve selected worktree for diverged push"), nil
		}
		if msg.err != nil {
			wt := m.cursorWorktree()
			if wt != nil && git.IsNonFastForwardPushError(msg.err) {
				return m.enterConfirm(forcePushPrompt(*wt), cmdStartPromptableJob(promptableJobForcePush, *wt, msg.log, false), "Force-pushing "+wt.Name+"…"), nil
			}
			return m.showError(msg.err.Error()), nil
		}
		cmds = append(cmds, notify.Info(ui.MessageComplete("push")))
		if wt := m.cursorWorktree(); wt != nil && wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadPreviewData(m.repo, *wt))
		}
		if msg.prURL != "" {
			prompt := fmt.Sprintf("Open pull request page?\n\n%s", msg.prURL)
			m = m.enterConfirmDefaultYes(prompt, cmdOpenURL(msg.prURL), "")
			return m, tea.Batch(cmds...)
		}
		return m, tea.Batch(cmds...)

	case pushFetchResultMsg:
		m.spinnerActive = false
		if msg.log != "" {
			m.lastJobLog = msg.log
			m.lastJobLabel = "Fetch output"
		}
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		div, err := git.DetectPushDivergenceAfterFetch(msg.wt.Path, msg.wt.Branch)
		if err != nil {
			return m.showError(err.Error()), nil
		}
		if div != nil {
			return m.enterPushDivergedMode(msg.wt, div), nil
		}
		m.spinnerActive = true
		m.spinnerLabel = promptableJobLabel(promptableJobPush, msg.wt)
		return m, tea.Batch(cmdStartPromptableJob(promptableJobPush, msg.wt, msg.log, false), m.spinner.Tick)

	case lazygitFinishedMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.loading = true
		return m, cmdLoadWorktrees(m.repo)

	case forcePushResultMsg:
		m.spinnerActive = false
		m.lastJobLog = msg.log
		m.lastJobLabel = "Force-push output"
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		cmds = append(cmds, notify.Info(ui.MessageComplete("force push")))
		if wt := m.cursorWorktree(); wt != nil && wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadPreviewData(m.repo, *wt))
		}
		return m, tea.Batch(cmds...)

	case pruneRemotesMsg:
		if msg.err != nil {
			return m.showError("remote prune failed: " + msg.err.Error()), nil
		}
		return m, nil

	case remoteUpdateResultMsg:
		m.spinnerActive = false
		m.lastJobLog = msg.log
		m.lastJobLabel = "Remote update output"
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.loading = true
		return m, cmdLoadWorktrees(m.repo)

	case trackResultMsg:
		m.spinnerActive = false
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		cmds = append(cmds, notify.Info("tracking remote branch"))
		if wt := m.cursorWorktree(); wt != nil && wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadPreviewData(m.repo, *wt))
		}
		return m, tea.Batch(cmds...)

	case pasteResultMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.clipboard = nil
		return m, tea.Batch(notify.Info(fmt.Sprintf("pasted %d file(s)", msg.n)), cmdLoadWorktrees(m.repo))

	case worktreesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			if m.ready {
				return m.showError(msg.err.Error()), nil
			}
			m.err = msg.err
			return m, nil
		}
		firstLoad := len(m.worktrees) == 0
		m.worktrees = sortedWorktrees(msg.worktrees, m.repo.MainBranch)
		m.dirties = make(map[string]dirtyState)
		m = m.resized()
		m.table.SetRows(m.buildRows())

		for i, wt := range m.worktrees {
			if wt.Path == m.activeWorktreePath {
				m.table.SetCursor(i)
				break
			}
		}

		for _, wt := range m.worktrees {
			if wt.Branch != "" {
				cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch))
				cmds = append(cmds, cmdLoadBaseStatus(m.repo, wt.Branch))
			}
			cmds = append(cmds, cmdLoadDirtyStatus(wt))
		}
		if len(m.worktrees) > 0 {
			if firstLoad {
				m.previewLoading = true
				m.viewport.SetContent(m.previewContent())
				cmds = append(cmds, m.spinner.Tick)
			}
			cmds = append(cmds, cmdLoadPreviewData(m.repo, m.worktrees[m.table.Cursor()]))
		}
		if m.refreshing {
			m.refreshing = false
			cmds = append(cmds, notify.Close("refresh"), notify.Success("refreshed"))
		}
		return m, tea.Batch(cmds...)

	case syncStatusMsg:
		m.statuses[msg.branch] = msg.status
		m.table.SetRows(m.buildRows())
		return m, nil

	case baseStatusMsg:
		rebased := msg.rebased
		m.baseStatus[msg.branch] = &rebased
		m.table.SetRows(m.buildRows())
		// Refresh preview if the updated branch belongs to the selected worktree
		if wt := m.cursorWorktree(); wt != nil && wt.Branch == msg.branch {
			m.viewport.SetContent(m.previewContent())
		}
		return m, nil

	case dirtyStatusMsg:
		m.dirties[msg.worktreePath] = msg.dirty
		m.table.SetRows(m.buildRows())
		return m, nil

	case previewDataMsg:
		if len(m.worktrees) > 0 && m.worktrees[m.table.Cursor()].Path == msg.worktreePath {
			m.previewUpstream = msg.upstream
			m.previewHeadCommit = msg.headCommit
			m.previewAheadCommits = msg.aheadCommits
			m.previewBehindCommits = msg.behindCommits
			m.previewChanges = msg.changes
			m.previewLoading = false
			m.viewport.SetContent(m.previewContent())
		}
		return m, nil
	}

	prevCursor := m.table.Cursor()

	var tableCmd tea.Cmd
	m.table, tableCmd = m.table.Update(msg)
	cmds = append(cmds, tableCmd)

	if m.table.Cursor() != prevCursor && len(m.worktrees) > 0 {
		m.table.SetRows(m.buildRows())
		m.previewLoading = true
		m.viewport.SetContent(m.previewContent())
		var spinnerCmd tea.Cmd
		if !m.spinnerActive {
			spinnerCmd = m.spinner.Tick
		}
		cmds = append(cmds, cmdLoadPreviewData(m.repo, m.worktrees[m.table.Cursor()]), spinnerCmd)
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}
