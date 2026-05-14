package status

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

const (
	bindingHelp          keys.BindingID = "help"
	bindingQuit          keys.BindingID = "quit"
	bindingGotoBottom    keys.BindingID = "goto-bottom"
	bindingGotoTop       keys.BindingID = "goto-top"
	bindingViewOutput    keys.BindingID = "view-output"
	bindingGotoLog       keys.BindingID = "goto-log"
	bindingGotoStatus    keys.BindingID = "goto-status"
	bindingGotoWorktrees keys.BindingID = "goto-worktrees"
	bindingGitCommit     keys.BindingID = "git-commit"
	bindingComment       keys.BindingID = "comment"
	bindingYankContent   keys.BindingID = "yank-content"
	bindingYankLocation  keys.BindingID = "yank-location"
	bindingYankAll       keys.BindingID = "yank-all"
	bindingYankFilename  keys.BindingID = "yank-filename"
	bindingLazygitLog    keys.BindingID = "lazygit-log"
	bindingRefreshMenu   keys.BindingID = "refresh-menu"
	bindingCancelChord   keys.BindingID = "cancel-chord"

	// Shared bindings: same action in both filetree and diff focus
	bindingToggleSection keys.BindingID = "toggle-section"
	bindingContextDec    keys.BindingID = "context-dec"
	bindingContextInc    keys.BindingID = "context-inc"
	bindingRefresh       keys.BindingID = "refresh"
	bindingRenderMode    keys.BindingID = "render-mode"
	bindingPull          keys.BindingID = "pull"
	bindingPush          keys.BindingID = "push"
	bindingRebase        keys.BindingID = "rebase"
	bindingAmend         keys.BindingID = "amend"
	bindingEdit          keys.BindingID = "edit"
)

func newStatusManager() keys.Manager {
	return keys.New([]keys.Binding{
		// Single-key globals
		{ID: bindingHelp, Seq: []string{"?"}, Categories: []string{"Global"}, Title: "help"},
		{ID: bindingQuit, Seq: []string{"q"}, Categories: []string{"Global"}, Title: "quit", Display: "q/ctrl+c"},
		{ID: bindingQuit, Seq: []string{"ctrl+c"}, Categories: []string{}, Title: ""},
		{ID: bindingLazygitLog, Seq: []string{"L"}, Categories: []string{"Global"}, Title: "lazygit log"},
		// G (shift+G) — register both forms since terminal encoding varies
		{ID: bindingGotoBottom, Seq: []string{"G"}, Categories: []string{"Filetree", "Diff"}, Title: "go to bottom", Display: "G"},
		{ID: bindingGotoBottom, Seq: []string{"shift+g"}, Categories: []string{}, Title: ""},

		// g-prefix chords
		{ID: bindingGotoTop, Seq: []string{"g", "g"}, Categories: []string{"Filetree", "Diff"}, Title: "go to top"},
		{ID: bindingViewOutput, Seq: []string{"g", "o"}, Categories: []string{"Global"}, Title: "view output"},
		{ID: bindingGotoLog, Seq: []string{"g", "l"}, Categories: []string{"Go to"}, Title: "goto log"},
		{ID: bindingGotoStatus, Seq: []string{"g", "s"}, Categories: []string{"Go to"}, Title: "goto status"},
		{ID: bindingGotoWorktrees, Seq: []string{"g", "w"}, Categories: []string{"Go to"}, Title: "goto worktrees"},
		{ID: bindingCancelChord, Seq: []string{"g", "esc"}, Categories: []string{}, Title: ""},

		// c-prefix chords
		{ID: bindingGitCommit, Seq: []string{"c", "c"}, Categories: []string{"Global"}, Title: "git commit"},
		{ID: bindingComment, Seq: []string{"c", "m"}, Categories: []string{"Diff"}, Title: "comment"},
		{ID: bindingCancelChord, Seq: []string{"c", "esc"}, Categories: []string{}, Title: ""},

		// y-prefix chords
		{ID: bindingYankContent, Seq: []string{"y", "y"}, Categories: []string{"Yank"}, Title: "yank content"},
		{ID: bindingYankLocation, Seq: []string{"y", "l"}, Categories: []string{"Yank"}, Title: "yank location"},
		{ID: bindingYankAll, Seq: []string{"y", "a"}, Categories: []string{"Yank"}, Title: "yank all"},
		{ID: bindingYankFilename, Seq: []string{"y", "f"}, Categories: []string{"Yank"}, Title: "yank filename"},
		{ID: bindingCancelChord, Seq: []string{"y", "esc"}, Categories: []string{}, Title: ""},

		// m-prefix chords
		{ID: bindingRefreshMenu, Seq: []string{"m", "r"}, Categories: []string{"Global"}, Title: "refresh menu"},
		{ID: bindingCancelChord, Seq: []string{"m", "esc"}, Categories: []string{}, Title: ""},

		// Shared single-key bindings (both filetree and diff focus)
		{ID: bindingToggleSection, Seq: []string{"tab"}, Categories: []string{"Diff"}, Title: "toggle staged/unstaged"},
		{ID: bindingContextDec, Seq: []string{"["}, Categories: []string{"Filetree", "Diff"}, Title: "fewer context lines"},
		{ID: bindingContextInc, Seq: []string{"]"}, Categories: []string{"Filetree", "Diff"}, Title: "more context lines"},
		{ID: bindingRefresh, Seq: []string{"R"}, Categories: []string{"Filetree", "Diff"}, Title: "refresh"},
		{ID: bindingRenderMode, Seq: []string{"s"}, Categories: []string{"Diff"}, Title: "toggle render mode"},
		{ID: bindingPull, Seq: []string{"p"}, Categories: []string{"Git"}, Title: "pull"},
		{ID: bindingPush, Seq: []string{"P"}, Categories: []string{"Git"}, Title: "push"},
		{ID: bindingRebase, Seq: []string{"b"}, Categories: []string{"Git"}, Title: "rebase"},
		{ID: bindingAmend, Seq: []string{"A"}, Categories: []string{"Git"}, Title: "amend"},
		{ID: bindingEdit, Seq: []string{"e"}, Categories: []string{"Filetree", "Diff"}, Title: "edit file"},
	})
}

func (m Model) dispatchBinding(id keys.BindingID, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch id {
	case bindingQuit:
		if m.runningRunner != nil && !m.runningDone {
			m.runningRunner.Cancel()
		}
		return m, tea.Quit
	case bindingLazygitLog:
		m.setStatus(ui.MessageOpening("lazygit log"))
		return m, cmdLazygitLog(m.worktreeRoot)
	case bindingGotoBottom:
		m.jumpToBottom()
		if m.focus == focusFiletree {
			return m, m.scheduleDiffReload()
		}
		return m, nil
	case bindingGotoTop:
		m.jumpToTop()
		m.clearStatus()
		if m.focus == focusFiletree {
			return m, m.scheduleDiffReload()
		}
		return m, nil
	case bindingViewOutput:
		if m.outputContent == "" {
			m.setStatus(ui.MessageNoOutput())
			return m, nil
		}
		m.openOutputModal()
		return m, nil
	case bindingGotoLog:
		if m.settings.EnableNavigation {
			m.clearStatus()
			return m, nav.Replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.worktreeRoot})
		}
		return m, nil
	case bindingGotoStatus:
		if m.settings.EnableNavigation {
			m.clearStatus()
			return m, nav.Replace(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: m.worktreeRoot})
		}
		return m, nil
	case bindingGotoWorktrees:
		if m.settings.EnableNavigation {
			m.clearStatus()
			return m, nav.Replace(nav.Route{Kind: nav.RouteWorktrees})
		}
		return m, nil
	case bindingGitCommit:
		m.setStatus(ui.MessageOpening("git commit"))
		return m, cmdGitCommit(m.worktreeRoot, m.settings.Terminal)
	case bindingComment:
		if m.focus != focusDiff {
			return m, nil
		}
		return m, m.cmdCreateCommentFromDiff()
	case bindingYankContent:
		m.yankContentOnly()
		return m, nil
	case bindingYankLocation:
		m.yankLocationOnly()
		return m, nil
	case bindingYankAll:
		m.yankAllContext()
		return m, nil
	case bindingYankFilename:
		m.yankFilename()
		return m, nil
	case bindingRefreshMenu:
		return m, m.refresh()
	case bindingCancelChord:
		m.clearStatus()
		return m, nil
	case bindingToggleSection:
		m.switchDiffSection()
		return m, nil
	case bindingContextDec:
		return m, m.adjustDiffContextLines(-1)
	case bindingContextInc:
		return m, m.adjustDiffContextLines(1)
	case bindingRefresh:
		return m, m.refresh()
	case bindingRenderMode:
		return m, m.toggleRenderMode()
	case bindingPull:
		if m.focus == focusFiletree {
			return m.startPullAction()
		}
		m.startPullAction()
		return m, actionPollCmd()
	case bindingPush:
		if err := m.preparePushConfirm(); err != nil {
			m.showGitError(err)
			return m, nil
		}
		return m, nil
	case bindingRebase:
		if err := m.prepareRebaseConfirm(); err != nil {
			m.showGitError(err)
			return m, nil
		}
		return m, nil
	case bindingAmend:
		if err := m.openAmendConfirm(); err != nil {
			m.showGitError(err)
		}
		return m, nil
	case bindingEdit:
		return m, m.cmdEditSelectedFile()
	}
	return m, nil
}
