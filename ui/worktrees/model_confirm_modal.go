package worktrees

import (
	"github.com/elentok/gx/ui/confirm"

	tea "charm.land/bubbletea/v2"
)

func (m Model) enterConfirm(prompt string, cmd tea.Cmd, spinnerLabel string) Model {
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:       prompt,
		AcceptCmd:    cmd,
		SpinnerLabel: spinnerLabel,
	})
	return m
}

func (m Model) enterConfirmWithCancel(prompt string, cmd tea.Cmd, spinnerLabel, cancelMsg string) Model {
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:       prompt,
		AcceptCmd:    cmd,
		SpinnerLabel: spinnerLabel,
		CancelMsg:    cancelMsg,
	})
	return m
}

func (m Model) enterConfirmWithItems(prompt string, items []string, cmd tea.Cmd, spinnerLabel string) Model {
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:       prompt,
		Items:        items,
		AcceptCmd:    cmd,
		SpinnerLabel: spinnerLabel,
	})
	return m
}

func (m Model) enterConfirmDefaultYes(prompt string, cmd tea.Cmd, spinnerLabel string) Model {
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:       prompt,
		AcceptCmd:    cmd,
		SpinnerLabel: spinnerLabel,
		DefaultYes:   true,
	})
	return m
}

func (m Model) handleConfirmUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.confirm.Update(msg)
	m.confirm = next
	if result.Done {
		m.mode = modeNormal
		if result.Accepted && result.SpinnerLabel != "" {
			m.spinnerActive = true
			m.spinnerLabel = result.SpinnerLabel
			return m, tea.Batch(cmd, m.spinner.Tick)
		}
		return m, cmd
	}
	return m, cmd
}

func (m Model) confirmModalView() string {
	return m.confirm.View(0)
}
