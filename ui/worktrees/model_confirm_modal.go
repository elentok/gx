package worktrees

import (
	"gx/ui"
	"gx/ui/components"

	tea "charm.land/bubbletea/v2"
)

// enterConfirm switches to confirm mode with the given prompt and the command
// to run if the user selects Yes. spinnerLabel, if non-empty, activates the
// spinner while the command runs.
func (m Model) enterConfirm(prompt string, cmd tea.Cmd, spinnerLabel string) Model {
	m.mode = modeConfirm
	m.confirmPrompt = prompt
	m.confirmYes = false
	m.confirmCmd = cmd
	m.confirmSpinnerLabel = spinnerLabel
	m.confirmCancelMsg = ""
	m.statusMsg = ""
	return m
}

func (m Model) enterConfirmWithCancel(prompt string, cmd tea.Cmd, spinnerLabel, cancelMsg string) Model {
	m = m.enterConfirm(prompt, cmd, spinnerLabel)
	m.confirmCancelMsg = cancelMsg
	return m
}

func (m Model) handleConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	nextYes, decided, accepted, handled := components.UpdateConfirm(msg, m.confirmYes)
	if !handled {
		return m, nil
	}
	m.confirmYes = nextYes
	if !decided {
		return m, nil
	}
	if accepted {
		return m.runConfirmed()
	}
	m.mode = modeNormal
	if m.confirmCancelMsg != "" {
		m.statusGen++
		m.statusMsg = m.confirmCancelMsg
		return m, cmdClearStatus(m.statusGen)
	}
	return m, nil
}

func (m Model) runConfirmed() (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	if m.confirmSpinnerLabel != "" {
		m.spinnerActive = true
		m.spinnerLabel = m.confirmSpinnerLabel
		return m, tea.Batch(m.confirmCmd, m.spinner.Tick)
	}
	return m, m.confirmCmd
}

func (m Model) confirmModalView() string {
	return components.RenderConfirmModal(
		m.confirmPrompt,
		m.confirmYes,
		ui.ColorBorder,
		ui.ColorGreen,
		ui.ColorRed,
		ui.ColorGray,
		0,
	)
}
