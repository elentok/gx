package worktrees

import (
	"fmt"

	"gx/git"
	"gx/ui/components"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func (m Model) Init() tea.Cmd {
	return cmdLoadWorktrees(m.repo)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m = m.resized()
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.mode {
		case modeError:
			return m.handleErrorKey(msg)
		case modeConfirm:
			return m.handleConfirmKey(msg)
		case modeRename:
			return m.handleRenameKey(msg)
		case modeClone:
			return m.handleCloneKey(msg)
		case modeNew, modeNewTmuxSession, modeNewTmuxWindow:
			return m.handleNewKey(msg)
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
		}
		if handledModel, cmd, handled := m.handleOutputKey(msg); handled {
			return handledModel, cmd
		}
		switch {
		case key.Matches(msg, keys.Search) && !m.spinnerActive:
			m = m.enterSearchMode()
			return m, nil
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			m = m.resized()
			return m, nil
		case key.Matches(msg, keys.New) && !m.spinnerActive:
			m = m.enterNewMode()
			return m, nil
		case key.Matches(msg, keys.NewTmuxSession) && !m.spinnerActive:
			m = m.enterNewTmuxSessionMode()
			return m, nil
		case key.Matches(msg, keys.NewTmuxWindow) && !m.spinnerActive:
			m = m.enterNewTmuxWindowMode()
			return m, nil
		case key.Matches(msg, keys.Delete) && len(m.worktrees) > 0 && !m.spinnerActive:
			return m.enterDeleteConfirm(), nil
		case key.Matches(msg, keys.Rename) && len(m.worktrees) > 0 && !m.spinnerActive:
			m = m.enterRenameMode()
			return m, nil
		case key.Matches(msg, keys.Clone) && len(m.worktrees) > 0 && !m.spinnerActive:
			m = m.enterCloneMode()
			return m, nil
		case key.Matches(msg, keys.Yank) && len(m.worktrees) > 0 && !m.spinnerActive:
			return m.enterYankMode()
		case key.Matches(msg, keys.Lazygit) && len(m.worktrees) > 0 && !m.spinnerActive:
			wt := m.selectedWorktree()
			if wt != nil {
				return m, cmdLazygit(*wt)
			}
		case key.Matches(msg, keys.Pull) && len(m.worktrees) > 0 && !m.spinnerActive:
			wt := m.selectedWorktree()
			if wt != nil {
				dirty := m.dirties[wt.Path]
				if dirty.hasModified || dirty.hasUntracked {
					return m.enterConfirmWithCancel(
						"Stash changes before pulling "+wt.Name+"?",
						cmdStashPull(*wt),
						"Pulling "+wt.Name+"…",
						"Pull aborted (dirty worktree)",
					), nil
				}
				m.spinnerActive = true
				m.spinnerLabel = "Pulling " + wt.Name + "…"
				return m, tea.Batch(cmdPull(*wt), m.spinner.Tick)
			}
		case key.Matches(msg, keys.Push) && len(m.worktrees) > 0 && !m.spinnerActive:
			wt := m.selectedWorktree()
			if wt != nil {
				if wt.Branch == "" {
					return m.showError("cannot push: worktree is in detached HEAD state"), nil
				}
				prompt := fmt.Sprintf("Push %s?", wt.Branch)
				return m.enterConfirm(prompt, cmdPush(m.repo, *wt), "Checking remote divergence…"), nil
			}
		case key.Matches(msg, keys.Rebase) && len(m.worktrees) > 0 && !m.spinnerActive:
			wt := m.selectedWorktree()
			if wt != nil {
				if wt.Branch == "" {
					return m.showError("cannot rebase: worktree is in detached HEAD state"), nil
				}
				if m.repo.MainBranch == "" {
					return m.showError("cannot rebase: no main branch detected"), nil
				}
				if wt.Branch == m.repo.MainBranch {
					return m.showError("cannot rebase: already on " + m.repo.MainBranch), nil
				}
				prompt := fmt.Sprintf("Rebase %s on %s?", wt.Branch, m.repo.MainBranch)
				dirty := m.dirties[wt.Path]
				if dirty.hasModified || dirty.hasUntracked {
					return m.enterConfirm(prompt, cmdRebasePreflight(m.repo, *wt), ""), nil
				}
				return m.enterConfirm(prompt, cmdRebase(m.repo, *wt, false), "Rebasing "+wt.Name+"…"), nil
			}
		case key.Matches(msg, keys.Refresh) && !m.spinnerActive:
			m.loading = true
			return m, cmdLoadWorktrees(m.repo)
		case key.Matches(msg, keys.RemoteUpdate) && !m.spinnerActive:
			m.spinnerActive = true
			m.spinnerLabel = "Fetching remotes…"
			return m, tea.Batch(cmdRemoteUpdate(m.repo), m.spinner.Tick)
		case key.Matches(msg, keys.Track) && len(m.worktrees) > 0 && !m.spinnerActive && m.sidebarUpstream == "":
			wt := m.selectedWorktree()
			if wt != nil {
				if wt.Branch == "" {
					return m.showError("cannot track: worktree is in detached HEAD state"), nil
				}
				return m.enterTrackConfirm(), nil
			}
		}

	case deleteResultMsg:
		m.spinnerActive = false
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.statusGen++
		m.statusMsg = fmt.Sprintf("Worktree %s deleted successfully", msg.name)
		return m, tea.Batch(cmdLoadWorktrees(m.repo), cmdClearStatus(m.statusGen))

	case renameResultMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.statusMsg = ""
		return m, cmdLoadWorktrees(m.repo)

	case cloneResultMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.statusMsg = ""
		return m, cmdLoadWorktrees(m.repo)

	case newResultMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.statusMsg = ""
		return m, cmdLoadWorktrees(m.repo)

	case newTmuxResultMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.statusMsg = ""
		var tmuxCmd tea.Cmd
		if msg.openMode == tmuxOpenWindow {
			tmuxCmd = cmdTmuxNewWindow(msg.name, msg.path)
		} else {
			tmuxCmd = cmdTmuxNewSession(msg.name, msg.path)
		}
		return m, tea.Batch(cmdLoadWorktrees(m.repo), tmuxCmd)

	case tmuxResultMsg:
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

	case clearStatusMsg:
		if msg.gen == m.statusGen {
			m.statusMsg = ""
		}
		return m, nil

	case spinner.TickMsg:
		if m.spinnerActive || m.sidebarLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			if m.sidebarLoading {
				m.viewport.SetContent(m.sidebarContent())
			}
			return m, cmd
		}
		return m, nil

	case pullResultMsg:
		m.spinnerActive = false
		m.lastJobLog = msg.log
		m.lastJobLabel = "Pull output"
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.statusGen++
		m.statusMsg = "Pulled"
		if msg.log != "" {
			m.statusMsg += "  ·  o  view output"
		}
		cmds = append(cmds, cmdClearStatus(m.statusGen))
		if wt := m.selectedWorktree(); wt != nil && wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadSidebarData(m.repo, *wt))
			if wt.Branch == m.repo.MainBranch {
				for _, w := range m.worktrees {
					if w.Branch != "" {
						cmds = append(cmds, cmdLoadBaseStatus(m.repo, w.Branch))
					}
				}
			}
		}
		return m, tea.Batch(cmds...)

	case stashPullStartedMsg:
		m.spinnerActive = false
		m.lastJobLog = msg.log
		m.lastJobLabel = "Pull output"
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.spinnerActive = true
		m.spinnerLabel = "Pulling " + msg.wt.Name + "…"
		return m, tea.Batch(cmdPullAfterStash(msg.wt, msg.log), m.spinner.Tick)

	case stashPullResultMsg:
		m.spinnerActive = false
		m.lastJobLog = msg.log
		m.lastJobLabel = "Pull output"
		if msg.err != nil {
			if msg.stashed {
				prompt := fmt.Sprintf("Pull failed: %s\n\nPop stash?", msg.err.Error())
				m = m.enterConfirm(prompt, cmdStashPop(msg.wtPath, "pull", msg.log), "Popping stash…")
				m.confirmYes = true
				return m, nil
			}
			return m.showError(msg.err.Error()), nil
		}
		if msg.stashed {
			m.spinnerActive = true
			m.spinnerLabel = "Popping stash…"
			return m, tea.Batch(cmdStashPop(msg.wtPath, "pull", msg.log), m.spinner.Tick)
		}
		// Should not reach here (stashPull always stashes), but handle gracefully
		m.statusGen++
		m.statusMsg = "Pulled"
		if msg.log != "" {
			m.statusMsg += "  ·  o  view output"
		}
		cmds = append(cmds, cmdClearStatus(m.statusGen))
		if wt := m.selectedWorktree(); wt != nil && wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadSidebarData(m.repo, *wt))
		}
		return m, tea.Batch(cmds...)

	case rebasePreflightMsg:
		return m.enterConfirmWithCancel(
			fmt.Sprintf("Stash changes before rebasing %s?", msg.wt.Branch),
			cmdRebase(msg.repo, msg.wt, true),
			"Rebasing "+msg.wt.Name+"…",
			"Rebase aborted (dirty worktree)",
		), nil

	case rebaseResultMsg:
		m.spinnerActive = false
		m.lastJobLog = msg.log
		m.lastJobLabel = "Rebase output"
		if msg.err != nil {
			if msg.stashed {
				prompt := fmt.Sprintf("Rebase failed: %s\n\nPop stash?", msg.err.Error())
				m = m.enterConfirm(prompt, cmdStashPop(msg.wtPath, "rebase", msg.log), "Popping stash…")
				m.confirmYes = true
				return m, nil
			}
			return m.showError(msg.err.Error()), nil
		}
		if msg.stashed {
			m.spinnerActive = true
			m.spinnerLabel = "Popping stash…"
			return m, tea.Batch(cmdStashPop(msg.wtPath, "rebase", msg.log), m.spinner.Tick)
		}
		m.statusGen++
		m.statusMsg = "Rebased"
		if msg.log != "" {
			m.statusMsg += "  ·  o  view output"
		}
		cmds = append(cmds, cmdClearStatus(m.statusGen))
		for _, w := range m.worktrees {
			if w.Branch != "" {
				cmds = append(cmds, cmdLoadBaseStatus(m.repo, w.Branch))
			}
		}
		if wt := m.selectedWorktree(); wt != nil && wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadSidebarData(m.repo, *wt))
		}
		return m, tea.Batch(cmds...)

	case stashPopResultMsg:
		m.spinnerActive = false
		m.lastJobLog = msg.log
		if msg.err != nil {
			return m.showError(fmt.Sprintf("Stash pop failed: %s", msg.err.Error())), nil
		}
		m.statusGen++
		switch msg.opLabel {
		case "pull":
			m.statusMsg = "Pulled (stash restored)"
		case "rebase":
			m.statusMsg = "Rebased (stash restored)"
		default:
			m.statusMsg = "Stash restored"
		}
		if m.lastJobLog != "" {
			m.statusMsg += "  ·  o  view output"
		}
		cmds = append(cmds, cmdClearStatus(m.statusGen))
		if wt := m.selectedWorktree(); wt != nil {
			cmds = append(cmds, cmdLoadDirtyStatus(*wt), cmdLoadSidebarData(m.repo, *wt))
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
			if wt := m.selectedWorktree(); wt != nil && wt.Branch == m.repo.MainBranch {
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
			wt := m.selectedWorktree()
			if wt != nil {
				return m.enterPushDivergedMode(*wt, msg.divergence), nil
			}
			return m.showError("cannot resolve selected worktree for diverged push"), nil
		}
		if msg.err != nil {
			wt := m.selectedWorktree()
			if wt != nil && git.IsNonFastForwardPushError(msg.err) {
				return m.enterConfirm(forcePushPrompt(*wt), cmdForcePush(m.repo, *wt, msg.log), "Force-pushing "+wt.Name+"…"), nil
			}
			return m.showError(msg.err.Error()), nil
		}
		m.statusGen++
		m.statusMsg = "Pushed"
		if msg.log != "" {
			m.statusMsg += "  ·  o  view output"
		}
		cmds = append(cmds, cmdClearStatus(m.statusGen))
		if wt := m.selectedWorktree(); wt != nil && wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadSidebarData(m.repo, *wt))
		}
		if msg.prURL != "" {
			prompt := fmt.Sprintf("Open pull request page?\n\n%s", msg.prURL)
			m = m.enterConfirm(prompt, cmdOpenURL(msg.prURL), "")
			m.confirmYes = true
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
		m.spinnerLabel = "Pushing " + msg.wt.Name + "…"
		return m, tea.Batch(cmdPushInteractive(m.repo, msg.wt, msg.log), m.spinner.Tick)

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
		m.statusGen++
		m.statusMsg = "Force-pushed"
		if msg.log != "" {
			m.statusMsg += "  ·  o  view output"
		}
		cmds = append(cmds, cmdClearStatus(m.statusGen))
		if wt := m.selectedWorktree(); wt != nil && wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadSidebarData(m.repo, *wt))
		}
		return m, tea.Batch(cmds...)

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
		m.statusGen++
		m.statusMsg = "Tracking remote branch"
		cmds = append(cmds, cmdClearStatus(m.statusGen))
		if wt := m.selectedWorktree(); wt != nil && wt.Branch != "" {
			cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadSidebarData(m.repo, *wt))
		}
		return m, tea.Batch(cmds...)

	case pasteResultMsg:
		if msg.err != nil {
			return m.showError(msg.err.Error()), nil
		}
		m.clipboard = nil
		m.statusGen++
		m.statusMsg = fmt.Sprintf("Pasted %d file(s)", msg.n)
		clearCmd := cmdClearStatus(m.statusGen)
		return m, tea.Batch(clearCmd, cmdLoadWorktrees(m.repo))

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
				m.sidebarLoading = true
				m.viewport.SetContent(m.sidebarContent())
				cmds = append(cmds, m.spinner.Tick)
			}
			cmds = append(cmds, cmdLoadSidebarData(m.repo, m.worktrees[m.table.Cursor()]))
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
		// Refresh sidebar if the updated branch belongs to the selected worktree
		if wt := m.selectedWorktree(); wt != nil && wt.Branch == msg.branch {
			m.viewport.SetContent(m.sidebarContent())
		}
		return m, nil

	case dirtyStatusMsg:
		m.dirties[msg.worktreePath] = msg.dirty
		m.table.SetRows(m.buildRows())
		return m, nil

	case sidebarDataMsg:
		if len(m.worktrees) > 0 && m.worktrees[m.table.Cursor()].Path == msg.worktreePath {
			m.sidebarUpstream = msg.upstream
			m.sidebarHeadCommit = msg.headCommit
			m.sidebarAheadCommits = msg.aheadCommits
			m.sidebarBehindCommits = msg.behindCommits
			m.sidebarChanges = msg.changes
			m.sidebarLoading = false
			m.viewport.SetContent(m.sidebarContent())
		}
		return m, nil
	}

	prevCursor := m.table.Cursor()

	var tableCmd tea.Cmd
	m.table, tableCmd = m.table.Update(msg)
	cmds = append(cmds, tableCmd)

	if m.table.Cursor() != prevCursor && len(m.worktrees) > 0 {
		m.table.SetRows(m.buildRows())
		m.sidebarLoading = true
		m.viewport.SetContent(m.sidebarContent())
		var spinnerCmd tea.Cmd
		if !m.spinnerActive {
			spinnerCmd = m.spinner.Tick
		}
		cmds = append(cmds, cmdLoadSidebarData(m.repo, m.worktrees[m.table.Cursor()]), spinnerCmd)
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}
