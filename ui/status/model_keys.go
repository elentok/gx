package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/status/diffarea"
	"github.com/elentok/gx/ui/terminalrun"
)

const (
	bindingHelp         keys.BindingID = "help"
	bindingQuit         keys.BindingID = "quit"
	bindingGotoBottom   keys.BindingID = "goto-bottom"
	bindingGotoTop      keys.BindingID = "goto-top"
	bindingViewOutput   keys.BindingID = "view-output"
	bindingGitCommit    keys.BindingID = "git-commit"
	bindingComment      keys.BindingID = "comment"
	bindingYankContent  keys.BindingID = "yank-content"
	bindingYankLocation keys.BindingID = "yank-location"
	bindingYankAll      keys.BindingID = "yank-all"
	bindingYankFilename keys.BindingID = "yank-filename"
	bindingLazygitLog   keys.BindingID = "lazygit-log"
	bindingRefreshMenu  keys.BindingID = "refresh-menu"
	bindingCancelChord  keys.BindingID = "cancel-chord"
	bindingGotoPR       keys.BindingID = "goto-pr"

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
	bindingBump          keys.BindingID = "bump"
	bindingEditInPlace   keys.BindingID = "edit"
	bindingEditHSplit    keys.BindingID = "edit-hsplit"
	bindingEditVSplit    keys.BindingID = "edit-vsplit"
	bindingEditTab       keys.BindingID = "edit-tab"
	bindingFilterLog     keys.BindingID = "filter-log"
	bindingStashAll      keys.BindingID = "stash-all"
	bindingStashStaged   keys.BindingID = "stash-staged"
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
		{ID: bindingGotoPR, Seq: []string{"g", "p"}, Categories: []string{"Go to"}, Title: "open PR"},
		{ID: bindingCancelChord, Seq: []string{"g", "esc"}, Categories: []string{}, Title: ""},

		// c-prefix chords
		{ID: bindingGitCommit, Seq: []string{"c", "c"}, Categories: []string{"Global"}, Title: "git commit"},
		{ID: bindingComment, Seq: []string{"c", "m"}, Categories: []string{"Diff"}, Title: "comment"},
		{ID: bindingCancelChord, Seq: []string{"c", "esc"}, Categories: []string{}, Title: ""},

		// y-prefix chords
		{ID: bindingYankContent, Seq: []string{"y", "y"}, Categories: []string{"Yank"}, Title: "yank content"},
		{ID: bindingYankLocation, Seq: []string{"y", "l"}, Categories: []string{"Yank"}, Title: "yank location"},
		{ID: bindingYankAll, Seq: []string{"y", "a"}, Categories: []string{"Yank"}, Title: "yank for AI agent"},
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
		{ID: bindingBump, Seq: []string{"B"}, Categories: []string{"Git"}, Title: "bump version"},
		// S-prefix chords (stash)
		{ID: bindingStashAll, Seq: []string{"S", "a"}, Categories: []string{"Git"}, Title: "stash all"},
		{ID: bindingStashStaged, Seq: []string{"S", "s"}, Categories: []string{"Git"}, Title: "stash staged"},
		{ID: bindingCancelChord, Seq: []string{"S", "esc"}, Categories: []string{}, Title: ""},
		// e-prefix chords
		{ID: bindingEditInPlace, Seq: []string{"e", "e"}, Categories: []string{"Filetree", "Diff"}, Title: "edit file"},
		{ID: bindingEditHSplit, Seq: []string{"e", "s"}, Categories: []string{"Filetree", "Diff"}, Title: "edit file (hsplit)"},
		{ID: bindingEditVSplit, Seq: []string{"e", "v"}, Categories: []string{"Filetree", "Diff"}, Title: "edit file (vsplit)"},
		{ID: bindingEditTab, Seq: []string{"e", "t"}, Categories: []string{"Filetree", "Diff"}, Title: "edit file (tab)"},
		{ID: bindingCancelChord, Seq: []string{"e", "esc"}, Categories: []string{}, Title: ""},
		{ID: bindingFilterLog, Seq: []string{"g", "h"}, Categories: []string{"Filetree", "Diff"}, Title: "log for file/hunk"},
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
		return m, tea.Batch(notify.Info(ui.MessageOpening("lazygit log")), cmdLazygitLog(m.worktreeRoot))
	case bindingGotoBottom:
		m.jumpToBottom()
		if m.focus == focusFiletree {
			return m, m.scheduleDiffReload()
		}
		return m, nil
	case bindingGotoTop:
		m.jumpToTop()
		if m.focus == focusFiletree {
			return m, m.scheduleDiffReload()
		}
		return m, nil
	case bindingViewOutput:
		if !m.output.HasContent() {
			return m, notify.Info(ui.MessageNoOutput())
		}
		m.openOutputModal()
		return m, nil
	case bindingGitCommit:
		return m, cmdGitCommit(m.worktreeRoot, m.settings.Terminal)
	case bindingComment:
		if m.focus != focusDiff {
			return m, nil
		}
		return m, m.cmdCreateCommentFromDiff()
	case bindingYankContent:
		return m, m.yankContentOnly()
	case bindingYankLocation:
		return m, m.yankLocationOnly()
	case bindingYankAll:
		return m, m.yankAllContext()
	case bindingYankFilename:
		return m, m.yankFilename()
	case bindingRefreshMenu:
		return m, tea.Batch(notify.Success("refreshed"), m.refresh())
	case bindingCancelChord:
		return m, nil
	case bindingToggleSection:
		m.switchDiffSection()
		return m, nil
	case bindingContextDec:
		return m, m.adjustDiffContextLines(-1)
	case bindingContextInc:
		return m, m.adjustDiffContextLines(1)
	case bindingRefresh:
		return m, tea.Batch(notify.Success("refreshed"), m.refresh())
	case bindingRenderMode:
		return m, m.toggleRenderMode()
	case bindingPull:
		cmd := m.pull.Open(m.worktreeRoot)
		return m, cmd
	case bindingPush:
		if err := m.push.Open(m.worktreeRoot); err != nil {
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
	case bindingBump:
		if err := m.bump.Open(m.worktreeRoot); err != nil {
			m.showGitError(err)
		}
		return m, nil
	case bindingStashAll:
		if !m.hasStashableChanges(false) {
			return m, notify.Info("nothing to stash")
		}
		m.keys.Reset()
		return m, m.stash.Open(m.worktreeRoot, false)
	case bindingStashStaged:
		if !m.hasStashableChanges(true) {
			return m, notify.Info("nothing staged to stash")
		}
		m.keys.Reset()
		return m, m.stash.Open(m.worktreeRoot, true)
	case bindingEditInPlace:
		return m, m.cmdEditSelectedFile(terminalrun.InPlace)
	case bindingEditHSplit:
		return m, m.cmdEditSelectedFile(terminalrun.HSplit)
	case bindingEditVSplit:
		return m, m.cmdEditSelectedFile(terminalrun.VSplit)
	case bindingEditTab:
		return m, m.cmdEditSelectedFile(terminalrun.Tab)
	case bindingFilterLog:
		if !m.settings.EnableNavigation {
			return m, nil
		}
		return m, nav.Open(m.filterLogViewState())
	case bindingGotoPR:
		return m, m.cmdGotoPR()
	}
	return m, nil
}

func (m Model) filterLogViewState() nav.ViewState {
	vs := nav.ViewState{Tab: nav.TabLog, WorktreeRoot: m.worktreeRoot}
	file, ok := m.selectedStatusFile()
	if !ok {
		return vs
	}
	vs.FilterPath = file.Path
	if m.focus == focusDiff {
		startLine, endLine := m.activeLogLineRange()
		vs.FilterStartLine = startLine
		vs.FilterEndLine = endLine
	}
	return vs
}

func (m Model) activeLogLineRange() (startLine, endLine int) {
	navMode := m.diffarea.NavMode()
	data := m.diffarea.SectionModel(diffarea.SectionUnstaged).Data()

	if navMode == diffview.NavModeHunk {
		if data.ActiveHunk >= 0 && data.ActiveHunk < len(data.Parsed.Hunks) {
			h := data.Parsed.Hunks[data.ActiveHunk]
			end := h.NewStart + h.NewCount - 1
			if end < h.NewStart {
				end = h.NewStart
			}
			return h.NewStart, end
		}
		return 0, 0
	}
	// NavModeLine: only changed lines have NewLine > 0
	if data.ActiveLine >= 0 && data.ActiveLine < len(data.Parsed.Changed) {
		cl := data.Parsed.Changed[data.ActiveLine]
		if cl.NewLine > 0 {
			return cl.NewLine, cl.NewLine
		}
	}
	return 0, 0
}
