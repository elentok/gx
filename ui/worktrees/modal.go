package worktrees

import (
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
	// apply runs against the page after close, for what only the page can do
	// (read the cursor worktree, move the table, open the next modal).
	apply func(Model) (Model, tea.Cmd)
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
	if res.apply != nil {
		var applyCmd tea.Cmd
		m, applyCmd = res.apply(m)
		cmd = tea.Batch(cmd, applyCmd)
	}
	return m, cmd, true
}
