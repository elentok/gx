package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/notify"
)

// queuePauseConfirmedMsg/queueResumeConfirmedMsg/budgetOverrideConfirmedMsg
// carry the "p" keymap's confirm-dialog acceptance, mirroring
// queueClearConfirmedMsg's shape in queue_clear.go.
type queuePauseConfirmedMsg struct{}
type queueResumeConfirmedMsg struct{}
type budgetOverrideConfirmedMsg struct{}

func cmdConfirmQueuePause() tea.Cmd {
	return func() tea.Msg { return queuePauseConfirmedMsg{} }
}

func cmdConfirmQueueResume() tea.Cmd {
	return func() tea.Msg { return queueResumeConfirmedMsg{} }
}

func cmdConfirmBudgetOverride() tea.Cmd {
	return func() tea.Msg { return budgetOverrideConfirmedMsg{} }
}

func (m QueueModel) handleQueuePauseConfirmed(_ queuePauseConfirmedMsg) (tea.Model, tea.Cmd) {
	ralphLoopRegistry.pause()
	m.paused = true
	return m, notify.Info("queue paused")
}

func (m QueueModel) handleQueueResumeConfirmed(_ queueResumeConfirmedMsg) (tea.Model, tea.Cmd) {
	ralphLoopRegistry.resume()
	m.paused = false
	return m, tea.Batch(notify.Success("queue resumed"), m.startAvailableEpics())
}

func (m QueueModel) handleBudgetOverrideConfirmed(_ budgetOverrideConfirmedMsg) (tea.Model, tea.Cmd) {
	if ralphLoopRegistry.isHardLimitPaused() {
		OverrideHardLimitPause()
	} else {
		OverrideSoftLimitPause()
	}
	return m, tea.Batch(notify.Success("budget pause overridden"), m.startAvailableEpics())
}
