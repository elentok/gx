package worktrees

import (
	"github.com/elentok/gx/ui/confirm"

	tea "charm.land/bubbletea/v2"
)

func (m Model) enterConfirm(opts confirm.Options) Model {
	m.confirm = m.confirm.Open(opts)
	return m
}

// applyConfirmResult stores the confirm dialog's next state and, when the
// user accepted a spinner-backed action, starts the spinner.
func applyConfirmResult(m Model, next confirm.Model, cmd tea.Cmd, result confirm.Result) (tea.Model, tea.Cmd) {
	m.confirm = next
	if result.Done {
		m.modal = nil
		if result.Accepted && result.SpinnerLabel != "" {
			m.spinnerActive = true
			m.spinnerLabel = result.SpinnerLabel
			return m, tea.Batch(cmd, m.spinner.Tick)
		}
	}
	return m, cmd
}
