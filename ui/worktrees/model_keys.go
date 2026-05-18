package worktrees

import (
	"fmt"

	"github.com/elentok/gx/ui"
	keymgr "github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

const (
	bindingSearch       keymgr.BindingID = "search"
	bindingBack         keymgr.BindingID = "back"
	bindingHelp         keymgr.BindingID = "help"
	bindingNew          keymgr.BindingID = "new"
	bindingNewAndOpen   keymgr.BindingID = "new-and-open"
	bindingDelete       keymgr.BindingID = "delete"
	bindingRename       keymgr.BindingID = "rename"
	bindingClone        keymgr.BindingID = "clone"
	bindingYank         keymgr.BindingID = "yank"
	bindingPull         keymgr.BindingID = "pull"
	bindingPush         keymgr.BindingID = "push"
	bindingRebase       keymgr.BindingID = "rebase"
	bindingRefresh      keymgr.BindingID = "refresh"
	bindingRemoteUpdate keymgr.BindingID = "remote-update"
	bindingTrack        keymgr.BindingID = "track"
	bindingOpenLog      keymgr.BindingID = "open-log"
	bindingLazygitLog   keymgr.BindingID = "lazygit-log"
	bindingOpenTerminal keymgr.BindingID = "open-terminal"

	bindingGotoTop       keymgr.BindingID = "goto-top"
	bindingGoOutput      keymgr.BindingID = "go-output"
	bindingRefreshMenu   keymgr.BindingID = "refresh-menu"
	bindingCancelChord   keymgr.BindingID = "cancel-chord"
)

func newWorktreesManager() keymgr.Manager {
	return keymgr.New([]keymgr.Binding{
		{ID: bindingSearch, Seq: []string{"/"}, Categories: []string{"Global"}, Title: "search"},
		{ID: bindingBack, Seq: []string{"q"}, Categories: []string{"Global"}, Title: "back / quit", Display: "q/esc/ctrl+c"},
		{ID: bindingBack, Seq: []string{"esc"}, Categories: []string{}, Title: ""},
		{ID: bindingBack, Seq: []string{"ctrl+c"}, Categories: []string{}, Title: ""},
		{ID: bindingHelp, Seq: []string{"?"}, Categories: []string{"Global"}, Title: "help"},
		{ID: bindingNew, Seq: []string{"n"}, Categories: []string{"Worktree"}, Title: "new worktree"},
		{ID: bindingNewAndOpen, Seq: []string{"N"}, Categories: []string{"Worktree"}, Title: "new worktree + open"},
		{ID: bindingDelete, Seq: []string{"d"}, Categories: []string{"Worktree"}, Title: "delete"},
		{ID: bindingRename, Seq: []string{"r"}, Categories: []string{"Worktree"}, Title: "rename"},
		{ID: bindingClone, Seq: []string{"c"}, Categories: []string{"Worktree"}, Title: "clone"},
		{ID: bindingYank, Seq: []string{"y"}, Categories: []string{"Worktree"}, Title: "yank files"},
		{ID: bindingPull, Seq: []string{"p"}, Categories: []string{"Git"}, Title: "pull"},
		{ID: bindingPush, Seq: []string{"P"}, Categories: []string{"Git"}, Title: "push"},
		{ID: bindingRebase, Seq: []string{"b"}, Categories: []string{"Git"}, Title: "rebase on main"},
		{ID: bindingRefresh, Seq: []string{"R"}, Categories: []string{"Git"}, Title: "refresh"},
		{ID: bindingRemoteUpdate, Seq: []string{"U"}, Categories: []string{"Git"}, Title: "remote update"},
		{ID: bindingTrack, Seq: []string{"t"}, Categories: []string{"Git"}, Title: "track"},
		{ID: bindingOpenLog, Seq: []string{"enter"}, Categories: []string{"Global"}, Title: "open log"},
		{ID: bindingLazygitLog, Seq: []string{"L"}, Categories: []string{"Global"}, Title: "lazygit log"},
		{ID: bindingOpenTerminal, Seq: []string{"o"}, Categories: []string{"Global"}, Title: "open in terminal"},

		{ID: bindingGotoTop, Seq: []string{"g", "g"}, Categories: []string{"Go to"}, Title: "top"},
		{ID: bindingGoOutput, Seq: []string{"g", "o"}, Categories: []string{"Go to"}, Title: "view output"},
		{ID: bindingCancelChord, Seq: []string{"g", "esc"}, Categories: []string{}, Title: ""},

		{ID: bindingRefreshMenu, Seq: []string{"m", "r"}, Categories: []string{"Global"}, Title: "refresh"},
		{ID: bindingCancelChord, Seq: []string{"m", "esc"}, Categories: []string{}, Title: ""},
	})
}

func (m Model) dispatchBinding(id keymgr.BindingID) (tea.Model, tea.Cmd) {
	switch id {
	case bindingSearch:
		if !m.spinnerActive {
			m = m.enterSearchMode()
		}
		return m, nil
	case bindingBack:
		if m.settings.EnableNavigation {
			return m, nav.Back()
		}
		return m, tea.Quit
	case bindingHelp:
		m = m.enterHelpMode()
		return m, nil
	case bindingNew:
		if !m.spinnerActive {
			m = m.enterNewMode()
		}
		return m, nil
	case bindingNewAndOpen:
		if !m.spinnerActive {
			m = m.enterNewAndOpenMode()
		}
		return m, nil
	case bindingDelete:
		if len(m.worktrees) > 0 && !m.spinnerActive {
			return m.enterDeleteConfirm(), nil
		}
		return m, nil
	case bindingRename:
		if len(m.worktrees) > 0 && !m.spinnerActive {
			m = m.enterRenameMode()
		}
		return m, nil
	case bindingClone:
		if len(m.worktrees) > 0 && !m.spinnerActive {
			m = m.enterCloneMode()
		}
		return m, nil
	case bindingYank:
		if len(m.worktrees) > 0 && !m.spinnerActive {
			return m.enterYankMode()
		}
		return m, nil
	case bindingPull:
		if len(m.worktrees) == 0 || m.spinnerActive {
			return m, nil
		}
		wt := m.selectedWorktree()
		if wt == nil {
			return m, nil
		}
		dirty := m.dirties[wt.Path]
		if dirty.hasModified || dirty.hasUntracked {
			return m.enterConfirmWithCancel(
				"Stash changes before pulling "+wt.Name+"?",
				cmdStashPull(*wt),
				"Pulling "+wt.Name+"…",
				"pull aborted (dirty worktree)",
			), nil
		}
		return m, cmdStartPromptableJob(promptableJobPull, *wt, "", false)
	case bindingPush:
		if len(m.worktrees) == 0 || m.spinnerActive {
			return m, nil
		}
		wt := m.selectedWorktree()
		if wt == nil {
			return m, nil
		}
		if wt.Branch == "" {
			return m.showError("cannot push: worktree is in detached HEAD state"), nil
		}
		prompt := fmt.Sprintf("Push %s?", wt.Branch)
		return m.enterConfirm(prompt, cmdStartPromptableJob(promptableJobPushFetch, *wt, "", false), "Checking remote divergence…"), nil
	case bindingRebase:
		if len(m.worktrees) == 0 || m.spinnerActive {
			return m, nil
		}
		wt := m.selectedWorktree()
		if wt == nil {
			return m, nil
		}
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
	case bindingRefresh, bindingRefreshMenu:
		if !m.spinnerActive {
			m.loading = true
			m.refreshing = true
			return m, tea.Batch(notify.Progress("refresh", "refreshing..."), cmdLoadWorktrees(m.repo), cmdPruneRemotes(m.repo))
		}
		return m, nil
	case bindingRemoteUpdate:
		if !m.spinnerActive {
			m.spinnerActive = true
			m.spinnerLabel = "Fetching remotes…"
			return m, tea.Batch(cmdRemoteUpdate(m.repo), m.spinner.Tick)
		}
		return m, nil
	case bindingTrack:
		if len(m.worktrees) == 0 || m.spinnerActive || m.sidebarUpstream != "" {
			return m, nil
		}
		wt := m.selectedWorktree()
		if wt == nil {
			return m, nil
		}
		if wt.Branch == "" {
			return m.showError("cannot track: worktree is in detached HEAD state"), nil
		}
		return m.enterTrackConfirm(), nil
	case bindingOpenLog:
		if len(m.worktrees) > 0 && m.settings.EnableNavigation {
			wt := m.selectedWorktree()
			if wt != nil {
				return m, nav.Replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: wt.Path})
			}
		}
		return m, nil
	case bindingLazygitLog:
		wt := m.selectedWorktree()
		if wt != nil {
			return m, tea.Batch(notify.Info(ui.MessageOpening("lazygit log")), cmdLazygitLog(*wt))
		}
		return m, nil
	case bindingOpenTerminal:
		if m.settings.Terminal == ui.TerminalPlain {
			return m, notify.Info("use tmux or kitty for more options")
		}
		wt := m.selectedWorktree()
		if wt != nil {
			return m.enterTerminalMenuFor(wt.Name, wt.Path), nil
		}
		return m, nil
	case bindingGotoTop:
		if len(m.worktrees) == 0 {
			return m, nil
		}
		m.table.SetCursor(0)
		return m, cmdLoadSidebarData(m.repo, m.worktrees[0])
	case bindingGoOutput:
		if m.lastJobLog == "" {
			return m, notify.Info(ui.MessageNoOutput())
		}
		return m.enterLogsMode(), nil
	case bindingCancelChord:
		return m, nil
	}
	return m, nil
}
