package worktrees

import (
	"github.com/elentok/gx/git"

	tea "charm.land/bubbletea/v2"
)

// modal is one dialog drawn over the worktrees page. Model.modal is nil in
// normal mode. A modal owns only its own state; what it wants from the page
// (close, a spinner, another modal) comes back in a modalResult and the root
// applies it.
type modal interface {
	Update(msg tea.Msg) (modal, tea.Cmd, modalResult)
	// View renders the modal, centred over the page. An empty view draws
	// nothing.
	View(width, height int) string
	InputFocused() bool
}

// bottomAnchored marks a modal drawn at the bottom of the page (where
// settings.InputModalBottom puts it) instead of centred.
type bottomAnchored interface {
	bottomAnchored()
}

// modalResult is what a modal asks the root to do after an update.
type modalResult struct {
	// unhandled sends the message on to the page's own handling (paste and
	// wheel events a modal ignores).
	unhandled bool
	// close dismisses the modal.
	close bool
	// openLogs replaces the modal with the last job's output.
	openLogs bool
	// spinner, when set, starts the busy spinner with this label.
	spinner string
	// action is what only the page can do after close (read the cursor
	// worktree, move the table, open the next modal). Nil when there is none.
	action modalAction
}

// modalAction is a closed set of requests a modal makes of the page. The root
// interprets each in applyModalAction.
type modalAction interface {
	modalAction()
}

type (
	// searchQueryChanged recomputes the matches and jumps to the current one.
	searchQueryChanged struct{}
	// searchCursorMoved jumps to the search's current match.
	searchCursorMoved struct{}
	// searchClosed clears the search highlights.
	searchClosed struct{}
	// pasteHere pastes the clipboard into the worktree under the cursor.
	pasteHere struct{ clipboard clipboardState }
	// moveCursor forwards a navigation key to the worktree table.
	moveCursor struct{ key tea.KeyPressMsg }
	// openPaste enters paste mode holding the clipboard.
	openPaste struct{ clipboard clipboardState }
	// submitName runs the command for a name entered in a text prompt.
	submitName struct {
		kind nameKind
		name string
	}
	// rebase rebases wt onto upstream.
	rebase struct {
		wt       git.Worktree
		upstream string
		log      string
	}
	// forcePush force-pushes wt.
	forcePush struct {
		wt  git.Worktree
		log string
	}
)

func (searchQueryChanged) modalAction() {}
func (searchCursorMoved) modalAction()  {}
func (searchClosed) modalAction()       {}
func (pasteHere) modalAction()          {}
func (moveCursor) modalAction()         {}
func (openPaste) modalAction()          {}
func (submitName) modalAction()         {}
func (rebase) modalAction()             {}
func (forcePush) modalAction()          {}

// nameKind says which command a text prompt's name feeds.
type nameKind int

const (
	nameRename nameKind = iota
	nameClone
	nameNew
	nameNewAndOpen
)

// applyModalAction runs a modal's action against the page.
func (m Model) applyModalAction(a modalAction) (Model, tea.Cmd) {
	switch a := a.(type) {
	case searchQueryChanged:
		return m.updateSearchMatches()
	case searchCursorMoved:
		return m.jumpToCurrentMatch()
	case searchClosed:
		return m.exitSearchMode(), nil
	case pasteHere:
		if wt := m.cursorWorktree(); wt != nil {
			return m, cmdPaste(a.clipboard, *wt)
		}
		return m, nil
	case moveCursor:
		return m.moveTableCursor(a.key)
	case openPaste:
		return m.enterPasteMode(a.clipboard)
	case submitName:
		return m.submitName(a)
	case rebase:
		return m, cmdRebaseRef(a.wt, a.upstream, a.log)
	case forcePush:
		return m, cmdStartPromptableJob(promptableJobForcePush, a.wt, a.log, false)
	}
	return m, nil
}

// moveTableCursor moves the table and reloads the preview when the cursor
// landed on another worktree.
func (m Model) moveTableCursor(k tea.KeyPressMsg) (Model, tea.Cmd) {
	prevCursor := m.table.Cursor()
	var tableCmd tea.Cmd
	m.table, tableCmd = m.table.Update(k)
	if m.table.Cursor() != prevCursor && len(m.worktrees) > 0 {
		m.table.SetRows(m.buildRows())
		m.previewLoading = true
		m.previewUpstream = ""
		m.previewAheadCommits = nil
		m.previewBehindCommits = nil
		m.previewChanges = nil
		m.viewport.SetContent(m.previewContent())
		return m, tea.Batch(tableCmd, cmdLoadPreviewData(m.repo, m.worktrees[m.table.Cursor()]))
	}
	return m, tableCmd
}

// submitName runs the command for a text prompt's name.
func (m Model) submitName(a submitName) (Model, tea.Cmd) {
	switch a.kind {
	case nameRename, nameClone:
		wt := m.cursorWorktree()
		if wt == nil || a.name == wt.Name {
			return m, nil
		}
		if a.kind == nameRename {
			return m, cmdRename(m.repo, *wt, a.name)
		}
		return m, cmdClone(m.repo, *wt, a.name)
	case nameNew, nameNewAndOpen:
		if m.repo.Trunk == "" {
			return m.showError("cannot create worktree: " + m.trunkProblem()), nil
		}
		if a.kind == nameNew {
			return m, cmdNewWorktree(m.repo, a.name)
		}
		return m, cmdNewWorktreeAndOpen(m.repo, a.name)
	}
	return m, nil
}

// openModal makes next the active modal.
func (m Model) openModal(next modal) Model {
	m.modal = next
	return m
}

// updateModal sends msg to the open modal and applies its result. ok is false
// when the modal left the message unhandled.
func (m Model) updateModal(msg tea.Msg) (next Model, cmd tea.Cmd, ok bool) {
	updated, cmd, res := m.modal.Update(msg)
	if res.unhandled {
		return m, nil, false
	}
	m.modal = updated
	if res.close {
		m.modal = nil
	}
	if res.openLogs {
		m = m.enterLogsMode()
	}
	if res.spinner != "" {
		m.spinnerActive = true
		m.spinnerLabel = res.spinner
		cmd = tea.Batch(cmd, m.spinner.Tick)
	}
	if res.action != nil {
		var applyCmd tea.Cmd
		m, applyCmd = m.applyModalAction(res.action)
		cmd = tea.Batch(cmd, applyCmd)
	}
	return m, cmd, true
}
