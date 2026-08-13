package tickets

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/notify"
)

// handleQueueSuggestedActionsKey applies the "m" keymap: opens the
// suggested-actions menu for the selected row, or toasts when its status has
// none. The Queue tab is otherwise read-only for selection (ticket 08); this
// is a deliberate, narrow exception for this one action.
func (m QueueModel) handleQueueSuggestedActionsKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedQueueRow()
	if !ok {
		return m, notify.Info("select a ticket to see its suggested actions")
	}
	items := suggestedActionItems(r.epic.RenderedStatus(r.ticket), r.ticket)
	if len(items) == 0 {
		return m, notify.Info("no suggested actions for this ticket")
	}
	prompt := fmt.Sprintf("Suggested actions for %q:", r.ticket.Title)
	m.actionsMenu = m.actionsMenu.Open(r.ticket.Path, r.epic.Name, r.ticket.DisplayNumber(), prompt, items)
	return m, nil
}

// handleQueueActionsMenuKey drives the open suggested-actions menu, mirroring
// the Tickets tab's handleActionsMenuKey.
func (m QueueModel) handleQueueActionsMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	next, result, handled := m.actionsMenu.Update(msg)
	if !handled {
		return m, nil
	}
	m.actionsMenu = next
	if !result.Done || !result.Accepted {
		return m, nil
	}
	if result.Action == actionInvestigate {
		return m, cmdLaunchInvestigate(m.worktreeRoot, result.EpicName, result.TicketID)
	}
	return m, cmdApplySuggestedAction(result.Path, result.Action, func() tea.Msg { return queueActionAppliedMsg{} })
}

// queueActionAppliedMsg reports that cmdApplySuggestedAction's write finished
// successfully, so updateInner can trigger the same reload cmdLoadQueue uses.
type queueActionAppliedMsg struct{}
