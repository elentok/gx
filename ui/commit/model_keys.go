package commit

import (
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

const (
	bindingHelp        keys.BindingID = "help"
	bindingQuit        keys.BindingID = "quit"
	bindingTab         keys.BindingID = "tab"
	bindingExpandBody  keys.BindingID = "expand-body"
	bindingToggleMode  keys.BindingID = "toggle-mode"
	bindingToggleWrap  keys.BindingID = "toggle-wrap"
	bindingDown        keys.BindingID = "down"
	bindingUp          keys.BindingID = "up"
	bindingScrollDown  keys.BindingID = "scroll-down"
	bindingScrollUp    keys.BindingID = "scroll-up"
	bindingPageDown    keys.BindingID = "page-down"
	bindingPageUp      keys.BindingID = "page-up"
	bindingNext        keys.BindingID = "next"
	bindingPrev        keys.BindingID = "prev"
	bindingBottom      keys.BindingID = "bottom"
	bindingEnter       keys.BindingID = "enter"
	bindingRight       keys.BindingID = "right"
	bindingLeft        keys.BindingID = "left"
	bindingRefresh     keys.BindingID = "refresh"
	bindingGotoTop     keys.BindingID = "goto-top"
	bindingGotoWT      keys.BindingID = "goto-worktrees"
	bindingGotoLog     keys.BindingID = "goto-log"
	bindingGotoStatus  keys.BindingID = "goto-status"
	bindingYankContent keys.BindingID = "yank-content"
	bindingYankLoc     keys.BindingID = "yank-location"
	bindingYankAll     keys.BindingID = "yank-all"
	bindingYankFile    keys.BindingID = "yank-filename"
	bindingComment     keys.BindingID = "comment"
	bindingRefreshMenu keys.BindingID = "refresh-menu"
	bindingCancelChord keys.BindingID = "cancel-chord"
	bindingAmend       keys.BindingID = "amend"
)

func newCommitManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingHelp, Seq: []string{"?"}, Categories: []string{"Global"}, Title: "help"},
		{ID: bindingQuit, Seq: []string{"q"}, Categories: []string{"Global"}, Title: "back / exit pane", Display: "q/esc"},
		{ID: bindingQuit, Seq: []string{"esc"}, Categories: []string{}, Title: ""},
		{ID: bindingTab, Seq: []string{"tab"}, Categories: []string{"Global"}, Title: "cycle pane"},
		{ID: bindingExpandBody, Seq: []string{"b"}, Categories: []string{"Header"}, Title: "toggle commit body"},
		{ID: bindingToggleMode, Seq: []string{"a"}, Categories: []string{"Diff"}, Title: "toggle hunk/line mode"},
		{ID: bindingToggleWrap, Seq: []string{"w"}, Categories: []string{"Diff"}, Title: "toggle wrap"},
		{ID: bindingDown, Seq: []string{"j"}, Categories: []string{"Navigation"}, Title: "down"},
		{ID: bindingDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
		{ID: bindingUp, Seq: []string{"k"}, Categories: []string{"Navigation"}, Title: "up"},
		{ID: bindingUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
		{ID: bindingScrollDown, Seq: []string{"J"}, Categories: []string{"Navigation"}, Title: "scroll down"},
		{ID: bindingScrollUp, Seq: []string{"K"}, Categories: []string{"Navigation"}, Title: "scroll up"},
		{ID: bindingPageDown, Seq: []string{"ctrl+d"}, Categories: []string{"Navigation"}, Title: "page down"},
		{ID: bindingPageUp, Seq: []string{"ctrl+u"}, Categories: []string{"Navigation"}, Title: "page up"},
		{ID: bindingNext, Seq: []string{"."}, Categories: []string{"Navigation"}, Title: "next file/commit"},
		{ID: bindingPrev, Seq: []string{","}, Categories: []string{"Navigation"}, Title: "prev file/commit"},
		{ID: bindingBottom, Seq: []string{"G"}, Categories: []string{"Navigation"}, Title: "bottom", Display: "G"},
		{ID: bindingBottom, Seq: []string{"shift+g"}, Categories: []string{}, Title: ""},
		{ID: bindingEnter, Seq: []string{"enter"}, Categories: []string{"Navigation"}, Title: "open / expand"},
		{ID: bindingRight, Seq: []string{"l"}, Categories: []string{"Navigation"}, Title: "open / expand", Display: "l/right"},
		{ID: bindingRight, Seq: []string{"right"}, Categories: []string{}, Title: ""},
		{ID: bindingLeft, Seq: []string{"h"}, Categories: []string{"Navigation"}, Title: "collapse / exit diff", Display: "h/left"},
		{ID: bindingLeft, Seq: []string{"left"}, Categories: []string{}, Title: ""},
		{ID: bindingRefresh, Seq: []string{"R"}, Categories: []string{"Global"}, Title: "refresh"},

		{ID: bindingGotoTop, Seq: []string{"g", "g"}, Categories: []string{"Go to"}, Title: "top"},
		{ID: bindingGotoWT, Seq: []string{"g", "w"}, Categories: []string{"Go to"}, Title: "goto worktrees"},
		{ID: bindingGotoLog, Seq: []string{"g", "l"}, Categories: []string{"Go to"}, Title: "goto log"},
		{ID: bindingGotoStatus, Seq: []string{"g", "s"}, Categories: []string{"Go to"}, Title: "goto status"},
		{ID: bindingCancelChord, Seq: []string{"g", "esc"}, Categories: []string{}, Title: ""},

		{ID: bindingYankContent, Seq: []string{"y", "y"}, Categories: []string{"Yank"}, Title: "yank content"},
		{ID: bindingYankLoc, Seq: []string{"y", "l"}, Categories: []string{"Yank"}, Title: "yank location"},
		{ID: bindingYankAll, Seq: []string{"y", "a"}, Categories: []string{"Yank"}, Title: "yank all"},
		{ID: bindingYankFile, Seq: []string{"y", "f"}, Categories: []string{"Yank"}, Title: "yank filename"},
		{ID: bindingCancelChord, Seq: []string{"y", "esc"}, Categories: []string{}, Title: ""},

		{ID: bindingRefreshMenu, Seq: []string{"m", "r"}, Categories: []string{"Global"}, Title: "refresh"},
		{ID: bindingCancelChord, Seq: []string{"m", "esc"}, Categories: []string{}, Title: ""},

		{ID: bindingComment, Seq: []string{"c", "m"}, Categories: []string{"Diff"}, Title: "comment"},
		{ID: bindingCancelChord, Seq: []string{"c", "esc"}, Categories: []string{}, Title: ""},

		{ID: bindingAmend, Seq: []string{"A"}, Categories: []string{"Actions"}, Title: "amend commit with staged changes"},
	})
}

func (m Model) dispatchBinding(id keys.BindingID) (tea.Model, tea.Cmd) {
	switch id {
	case bindingHelp:
		m.keys.Reset()
		m.help.Open(m.width, m.height)
		return m, nil
	case bindingQuit:
		if m.search.IsActive() {
			m.search.DismissAndClear()
			return m, nil
		}
		if m.focusDiff {
			m.focusDiff = false
			return m, nil
		}
		if m.focusHeader {
			m.focusHeader = false
			return m, nil
		}
		return m, nav.Back()
	case bindingTab:
		return m.handleTabFocusCycle()
	case bindingExpandBody:
		m.bodyExpanded = !m.bodyExpanded
		m.scrollHeader(0)
		m.syncDiffViewport()
		return m, nil
	case bindingToggleMode:
		if !m.focusDiff {
			return m, nil
		}
		if m.diffModel.NavMode() == diffview.NavModeHunk {
			m.diffModel.SetNavMode(diffview.NavModeLine)
		} else {
			m.diffModel.SetNavMode(diffview.NavModeHunk)
		}
		m.ensureActiveVisible()
		return m, nil
	case bindingToggleWrap:
		if !m.focusDiff {
			return m, nil
		}
		m.diffModel.EnableWrap(!m.diffModel.WrapEnabled())
		m.syncDiffViewport()
		return m, nil
	case bindingDown:
		if m.focusHeader {
			m.scrollHeader(1)
			return m, nil
		}
		if m.focusDiff {
			m.moveDiffActive(1)
			return m, nil
		}
		m.moveSidebar(1)
		return m, nil
	case bindingUp:
		if m.focusHeader {
			m.scrollHeader(-1)
			return m, nil
		}
		if m.focusDiff {
			m.moveDiffActive(-1)
			return m, nil
		}
		m.moveSidebar(-1)
		return m, nil
	case bindingScrollDown:
		if m.focusHeader {
			m.scrollHeader(1)
		} else if m.focusDiff {
			m.diffModel.ScrollViewport(3)
		}
		return m, nil
	case bindingScrollUp:
		if m.focusHeader {
			m.scrollHeader(-1)
		} else if m.focusDiff {
			m.diffModel.ScrollViewport(-3)
		}
		return m, nil
	case bindingPageDown:
		if m.focusHeader {
			m.scrollHeaderPage(1)
		} else if m.focusDiff {
			m.scrollDiffPage(1)
		} else {
			m.scrollSidebarPage(1)
		}
		return m, nil
	case bindingPageUp:
		if m.focusHeader {
			m.scrollHeaderPage(-1)
		} else if m.focusDiff {
			m.scrollDiffPage(-1)
		} else {
			m.scrollSidebarPage(-1)
		}
		return m, nil
	case bindingNext:
		if m.focusDiff {
			m.moveToAdjacentFile(1)
		} else {
			m.moveToAdjacentCommit(-1)
		}
		return m, nil
	case bindingPrev:
		if m.focusDiff {
			m.moveToAdjacentFile(-1)
		} else {
			m.moveToAdjacentCommit(1)
		}
		return m, nil
	case bindingBottom:
		if m.focusDiff {
			m.jumpDiffBottom()
		} else {
			m.jumpSidebarBottom()
		}
		return m, nil
	case bindingEnter:
		if m.focusHeader {
			m.focusHeader = false
			return m, nil
		}
		if m.toggleDirOnEnter() {
			return m, nil
		}
		if _, ok := m.selectedCommitFile(); ok {
			m.focusDiff = true
			m.ensureActiveVisible()
		}
		return m, nil
	case bindingRight:
		if m.focusHeader {
			m.focusHeader = false
			return m, nil
		}
		if !m.focusDiff && m.expandSelectedDir() {
			return m, nil
		}
		if _, ok := m.selectedCommitFile(); ok {
			m.focusDiff = true
			m.ensureActiveVisible()
		}
		return m, nil
	case bindingLeft:
		if m.focusDiff {
			m.focusDiff = false
			return m, nil
		}
		if m.focusHeader {
			m.focusHeader = false
			return m, nil
		}
		if !m.focusDiff && m.collapseSelectedDir() {
			return m, nil
		}
		if m.fileTreeModel.FocusParent() {
			m.refreshDiff()
		}
		return m, nil
	case bindingRefresh, bindingRefreshMenu:
		m.reload()
		return m, nil
	case bindingGotoTop:
		if m.focusDiff {
			m.jumpDiffTop()
		} else {
			m.jumpSidebarTop()
		}
		return m, nil
	case bindingGotoWT:
		return m, nav.Replace(nav.Route{Kind: nav.RouteWorktrees})
	case bindingGotoLog:
		return m, nav.Replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.worktreeRoot, Ref: m.ref})
	case bindingGotoStatus:
		return m, nav.Replace(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: m.worktreeRoot})
	case bindingYankContent:
		if m.focusHeader {
			m.yankCommitBody()
		} else {
			m.yankContentOnly()
		}
		return m, nil
	case bindingYankLoc:
		m.yankLocationOnly()
		return m, nil
	case bindingYankAll:
		m.yankAllContext()
		return m, nil
	case bindingYankFile:
		m.yankFilename()
		return m, nil
	case bindingComment:
		if !m.focusDiff {
			return m, nil
		}
		return m, m.cmdCreateCommentFromDiff()
	case bindingAmend:
		if err := m.openAmendConfirm(); err != nil {
			m.statusMsg = err.Error()
		}
		return m, nil
	case bindingCancelChord:
		m.clearStatus()
		return m, nil
	}
	return m, nil
}

func (m Model) handleTabFocusCycle() (tea.Model, tea.Cmd) {
	if m.focusHeader {
		m.focusHeader = false
		m.focusDiff = false
		return m, nil
	}
	if _, ok := m.selectedCommitFile(); !ok {
		m.focusHeader = true
		m.focusDiff = false
		return m, nil
	}
	if m.focusDiff {
		m.focusDiff = false
		m.focusHeader = true
	} else {
		m.focusDiff = true
		m.focusHeader = false
		m.ensureActiveVisible()
	}
	return m, nil
}
