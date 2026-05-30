package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

const (
	bindingHelp        keys.BindingID = "help"
	bindingQuit        keys.BindingID = "quit"
	bindingTab         keys.BindingID = "tab"
	bindingExpandBody  keys.BindingID = "expand-body"
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
	bindingYankContent keys.BindingID = "yank-content"
	bindingYankLoc     keys.BindingID = "yank-location"
	bindingYankAll     keys.BindingID = "yank-all"
	bindingYankFile    keys.BindingID = "yank-filename"
	bindingYankHash    keys.BindingID = "yank-hash"
	bindingYankSubject keys.BindingID = "yank-subject"
	bindingYankMessage keys.BindingID = "yank-message"
	bindingComment     keys.BindingID = "comment"
	bindingRefreshMenu keys.BindingID = "refresh-menu"
	bindingCancelChord keys.BindingID = "cancel-chord"
	bindingAmend       keys.BindingID = "amend"
	bindingReword      keys.BindingID = "reword"
	bindingFilterLog   keys.BindingID = "filter-log"
	bindingEditInPlace keys.BindingID = "edit"
	bindingEditHSplit  keys.BindingID = "edit-hsplit"
	bindingEditVSplit  keys.BindingID = "edit-vsplit"
	bindingEditTab     keys.BindingID = "edit-tab"
	bindingContextDec  keys.BindingID = "context-dec"
	bindingContextInc  keys.BindingID = "context-inc"
	bindingGotoPR      keys.BindingID = "goto-pr"
)

func newCommitManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingHelp, Seq: []string{"?"}, Categories: []string{"Global"}, Title: "help"},
		{ID: bindingQuit, Seq: []string{"q"}, Categories: []string{"Global"}, Title: "back / exit pane", Display: "q/esc"},
		{ID: bindingQuit, Seq: []string{"esc"}, Categories: []string{}, Title: ""},
		{ID: bindingTab, Seq: []string{"tab"}, Categories: []string{"Global"}, Title: "cycle pane"},
		{ID: bindingExpandBody, Seq: []string{"b"}, Categories: []string{"Header"}, Title: "toggle commit body"},
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
		{ID: bindingGotoPR, Seq: []string{"g", "p"}, Categories: []string{"Go to"}, Title: "open PR"},
		{ID: bindingCancelChord, Seq: []string{"g", "esc"}, Categories: []string{}, Title: ""},

		{ID: bindingYankContent, Seq: []string{"y", "y"}, Categories: []string{"Yank"}, Title: "yank content"},
		{ID: bindingYankLoc, Seq: []string{"y", "l"}, Categories: []string{"Yank"}, Title: "yank location"},
		{ID: bindingYankAll, Seq: []string{"y", "a"}, Categories: []string{"Yank"}, Title: "yank for AI agent"},
		{ID: bindingYankFile, Seq: []string{"y", "f"}, Categories: []string{"Yank"}, Title: "yank filename"},
		{ID: bindingYankHash, Seq: []string{"y", "h"}, Categories: []string{"Yank"}, Title: "yank commit hash"},
		{ID: bindingYankSubject, Seq: []string{"y", "s"}, Categories: []string{"Yank"}, Title: "yank commit subject"},
		{ID: bindingYankMessage, Seq: []string{"y", "m"}, Categories: []string{"Yank"}, Title: "yank commit message"},
		{ID: bindingCancelChord, Seq: []string{"y", "esc"}, Categories: []string{}, Title: ""},

		{ID: bindingRefreshMenu, Seq: []string{"m", "r"}, Categories: []string{"Global"}, Title: "refresh"},
		{ID: bindingCancelChord, Seq: []string{"m", "esc"}, Categories: []string{}, Title: ""},

		{ID: bindingComment, Seq: []string{"c", "m"}, Categories: []string{"Diff"}, Title: "comment"},
		{ID: bindingReword, Seq: []string{"r", "w"}, Categories: []string{"Actions"}, Title: "reword commit"},
		{ID: bindingCancelChord, Seq: []string{"c", "esc"}, Categories: []string{}, Title: ""},

		{ID: bindingAmend, Seq: []string{"A"}, Categories: []string{"Actions"}, Title: "amend commit with staged changes"},
		{ID: bindingFilterLog, Seq: []string{"g", "h"}, Categories: []string{"Navigation"}, Title: "log for file/hunk"},

		{ID: bindingContextDec, Seq: []string{"["}, Categories: []string{"Diff"}, Title: "fewer context lines"},
		{ID: bindingContextInc, Seq: []string{"]"}, Categories: []string{"Diff"}, Title: "more context lines"},

		// e-prefix chords
		{ID: bindingEditInPlace, Seq: []string{"e", "e"}, Categories: []string{"Actions"}, Title: "edit file"},
		{ID: bindingEditHSplit, Seq: []string{"e", "s"}, Categories: []string{"Actions"}, Title: "edit file (hsplit)"},
		{ID: bindingEditVSplit, Seq: []string{"e", "v"}, Categories: []string{"Actions"}, Title: "edit file (vsplit)"},
		{ID: bindingEditTab, Seq: []string{"e", "t"}, Categories: []string{"Actions"}, Title: "edit file (tab)"},
		{ID: bindingCancelChord, Seq: []string{"e", "esc"}, Categories: []string{}, Title: ""},
	})
}

func (m Model) dispatchBinding(id keys.BindingID) (tea.Model, tea.Cmd) {
	switch id {
	case bindingHelp:
		m.keys.Reset()
		m.help.Open(m.width, m.height)
		return m, nil
	case bindingQuit:
		if m.focusDiff && m.diffModel.Search().IsActive() {
			m.diffModel.Search().DismissAndClear()
			return m, nil
		}
		if !m.focusDiff && m.fileTreeModel.Search().IsActive() {
			m.fileTreeModel.Search().DismissAndClear()
			return m, nil
		}
		if m.focusDiff && m.diffModel.DataRef().VisualActive {
			m.diffModel.DisableVisual()
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
	case bindingDown:
		if m.focusHeader {
			m.scrollHeader(1)
		}
		return m, nil
	case bindingUp:
		if m.focusHeader {
			m.scrollHeader(-1)
		}
		return m, nil
	case bindingScrollDown:
		if m.focusHeader {
			m.scrollHeader(1)
		}
		return m, nil
	case bindingScrollUp:
		if m.focusHeader {
			m.scrollHeader(-1)
		}
		return m, nil
	case bindingPageDown:
		if m.focusHeader {
			m.scrollHeaderPage(1)
		} else {
			m.scrollSidebarPage(1)
		}
		return m, nil
	case bindingPageUp:
		if m.focusHeader {
			m.scrollHeaderPage(-1)
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
		m.jumpSidebarBottom()
		return m, nil
	case bindingEnter:
		if m.focusHeader {
			m.focusHeader = false
		}
		return m, nil
	case bindingRight:
		if m.focusHeader {
			m.focusHeader = false
		}
		return m, nil
	case bindingLeft:
		if m.focusDiff {
			m.focusDiff = false
			return m, nil
		}
		if m.focusHeader {
			m.focusHeader = false
		}
		return m, nil
	case bindingRefresh, bindingRefreshMenu:
		m.reload()
		return m, notify.Success("refreshed")
	case bindingGotoTop:
		m.jumpSidebarTop()
		return m, nil
	case bindingYankContent:
		if m.focusHeader {
			return m, m.yankCommitBody()
		}
		return m, m.yankContentOnly()
	case bindingYankLoc:
		return m, m.yankLocationOnly()
	case bindingYankAll:
		return m, m.yankAllContext()
	case bindingYankFile:
		return m, m.yankFilename()
	case bindingYankHash:
		return m, m.yankCommitHash()
	case bindingYankSubject:
		return m, m.yankCommitSubject()
	case bindingYankMessage:
		return m, m.yankCommitMessage()
	case bindingComment:
		if !m.focusDiff {
			return m, nil
		}
		return m, m.cmdCreateCommentFromDiff()
	case bindingAmend:
		if err := m.openAmendConfirm(); err != nil {
			return m, notify.Error(err.Error())
		}
		return m, nil
	case bindingReword:
		return m, m.openRewordEditor()
	case bindingCancelChord:
		return m, nil
	case bindingFilterLog:
		return m, nav.Open(m.filterLogViewState())
	case bindingEditInPlace:
		return m, m.cmdEditSelectedFile(terminalrun.InPlace)
	case bindingEditHSplit:
		return m, m.cmdEditSelectedFile(terminalrun.HSplit)
	case bindingEditVSplit:
		return m, m.cmdEditSelectedFile(terminalrun.VSplit)
	case bindingEditTab:
		return m, m.cmdEditSelectedFile(terminalrun.Tab)
	case bindingContextDec:
		return m, m.adjustDiffContextLines(-1)
	case bindingContextInc:
		return m, m.adjustDiffContextLines(1)
	case bindingGotoPR:
		return m, m.cmdGotoPR()
	}
	return m, nil
}

func (m Model) filterLogViewState() nav.ViewState {
	vs := nav.ViewState{Tab: nav.TabLog, WorktreeRoot: m.worktreeRoot}
	file, ok := m.selectedCommitFile()
	if !ok {
		return vs
	}
	vs.FilterPath = file.Path
	if m.focusDiff {
		startLine, endLine := m.activeLogLineRange()
		vs.FilterStartLine = startLine
		vs.FilterEndLine = endLine
	}
	return vs
}

func (m Model) activeLogLineRange() (startLine, endLine int) {
	data := m.diffModel.Data()
	navMode := m.diffModel.NavMode()

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
	if data.ActiveLine >= 0 && data.ActiveLine < len(data.Parsed.Changed) {
		cl := data.Parsed.Changed[data.ActiveLine]
		if cl.NewLine > 0 {
			return cl.NewLine, cl.NewLine
		}
	}
	return 0, 0
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
