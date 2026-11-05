package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/tickets/queue"
)

// handleServerApproveKey is "A" in server mode, and the Approve entry of the
// ticket menu: confirm the selected ticket's pending recovery proposal.
func (m QueueModel) handleServerApproveKey() (tea.Model, tea.Cmd) {
	if row, ok := m.selectedQueueRow(); ok {
		if opts, ok := m.srv.ApproveConfirm(row.Ticket); ok {
			m.confirm = m.confirm.Open(opts)
		}
	}
	return m, nil
}

// handleServerFinishAndPauseKey is "D" in server mode, an alias of Finish and pause kept
// for one release: stop the server handing out new work; running tickets finish.
func (m Model) handleServerFinishAndPauseKey() (tea.Model, tea.Cmd) {
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    queue.FinishAndPausePrompt,
		AcceptCmd: queue.CmdServerQueueMode("queue paused", m.queue.Pause),
	})
	return m, nil
}
