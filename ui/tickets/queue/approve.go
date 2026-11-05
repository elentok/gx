package queue

import (
	tea "charm.land/bubbletea/v2"
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
