package worktrees

import (
	"path/filepath"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/worktrees/modal"

	tea "charm.land/bubbletea/v2"
)

// showError opens the error modal.
func (m Model) showError(errMsg string) Model {
	return m.openModal(modal.NewError(errMsg, m.width, m.height, m.lastJobLog != ""))
}

// enterLogsMode opens the logs modal on the last job output.
func (m Model) enterLogsMode() Model {
	return m.openModal(modal.NewLogs(m.lastJobLog, m.lastJobLabel, m.width, m.height))
}

// credentialOpen reports whether the credential modal is the open modal.
func credentialOpen(open modal.Modal) bool {
	_, ok := open.(modal.Credential)
	return ok
}

// herdrTerminalDown reports a herdr terminal whose herdr is missing or
// unreachable: its splits and tabs would fail, so the menu stays hidden.
func herdrTerminalDown(s ui.Settings) bool {
	return s.Terminal == ui.TerminalHerdr && s.HerdrUnavailable
}

func (m Model) enterTerminalMenuFor(name, path string) Model {
	title := "Open in Terminal"
	if wt := m.cursorWorktree(); wt != nil {
		title = "Open: " + wt.Name
	}
	return m.openModal(modal.NewTerminalMenu(
		m.settings.Terminal, name, path, title,
		filepath.Base(m.repo.LinkedWorktreeDir()), m.settings.NameAliases,
	))
}

// applyModalAction runs a modal's action against the page.
func (m Model) applyModalAction(a modal.Action) (Model, tea.Cmd) {
	switch a := a.(type) {
	case modal.SearchQueryChanged:
		return m.updateSearchMatches()
	case modal.SearchCursorMoved:
		return m.jumpToCurrentMatch()
	case modal.SearchClosed:
		return m.exitSearchMode(), nil
	case modal.PasteHere:
		if wt := m.cursorWorktree(); wt != nil {
			return m, cmdPaste(a.Clipboard, *wt)
		}
		return m, nil
	case modal.MoveCursor:
		return m.moveTableCursor(a.Key)
	case modal.OpenPaste:
		return m.enterPasteMode(a.Clipboard)
	case modal.SubmitName:
		return m.submitName(a)
	case modal.Rebase:
		return m, cmdRebaseRef(a.Wt, a.Upstream, a.Log)
	case modal.ForcePush:
		return m, cmdStartPromptableJob(promptableJobForcePush, a.Wt, a.Log, false)
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
func (m Model) submitName(a modal.SubmitName) (Model, tea.Cmd) {
	switch a.Kind {
	case modal.NameRename, modal.NameClone:
		wt := m.cursorWorktree()
		if wt == nil || a.Name == wt.Name {
			return m, nil
		}
		if a.Kind == modal.NameRename {
			return m, cmdRename(m.repo, *wt, a.Name)
		}
		return m, cmdClone(m.repo, *wt, a.Name)
	case modal.NameNew, modal.NameNewAndOpen:
		if m.repo.Trunk == "" {
			return m.showError("cannot create worktree: " + m.trunkProblem()), nil
		}
		if a.Kind == modal.NameNew {
			return m, cmdNewWorktree(m.repo, a.Name)
		}
		return m, cmdNewWorktreeAndOpen(m.repo, a.Name)
	}
	return m, nil
}

// openModal makes next the active modal.
func (m Model) openModal(next modal.Modal) Model {
	m.modal = next
	return m
}

// updateModal sends msg to the open modal and applies its result. ok is false
// when the modal left the message unhandled.
func (m Model) updateModal(msg tea.Msg) (next Model, cmd tea.Cmd, ok bool) {
	updated, cmd, res := m.modal.Update(msg)
	if res.Unhandled {
		return m, nil, false
	}
	m.modal = updated
	if res.Close {
		m.modal = nil
	}
	if res.OpenLogs {
		m = m.enterLogsMode()
	}
	if res.Spinner != "" {
		m.spinnerActive = true
		m.spinnerLabel = res.Spinner
		cmd = tea.Batch(cmd, m.spinner.Tick)
	}
	if res.Action != nil {
		var applyCmd tea.Cmd
		m, applyCmd = m.applyModalAction(res.Action)
		cmd = tea.Batch(cmd, applyCmd)
	}
	return m, cmd, true
}
