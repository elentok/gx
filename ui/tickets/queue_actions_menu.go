package tickets

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

// handleQueueSuggestedActionsKey applies the "m" keymap: opens the
// suggested-actions menu for the selected row, whatever its status. The Queue tab is otherwise read-only for selection (ticket 08); this
// is a deliberate, narrow exception for this one action.
func (m QueueModel) handleQueueSuggestedActionsKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedQueueRow()
	if !ok {
		return m, notify.Info("select a ticket to see its suggested actions")
	}
	status := r.epic.RenderedStatus(r.ticket)
	paneEpic, nativeLive := r.epic.Name, false
	if _, address := resolveTicketPath(m.settings.TicketStore, r.ticket.Path); address != "" {
		// A server-mode row's epic is "project:epic"; the herdr panes are named
		// by the bare epic. Only an address locates a native agent.
		paneEpic, _, _ = tickets.SplitTrailerValue(address)
		nativeLive = nativeAgentLive(address)
	}
	paneLive := !m.settings.HerdrUnavailable && ticketPaneLive(paneEpic, r.ticket.DisplayNumber())
	items := suggestedActionItems(status, r.ticket, paneLive, nativeLive, m.settings.HerdrUnavailable)
	if m.serverAPI != nil && isParkedRoot(r.epic, m.checked) {
		items = append([]components.MenuItem{mergeMenuItem}, items...)
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
	if result.Action == actionMerge {
		return m, cmdEpicMerge(m.serverAPI, result.EpicName)
	}
	if result.Action == actionInvestigate {
		return m, cmdLaunchInvestigate(m.worktreeRoot, result.EpicName, result.TicketID)
	}
	file, address := resolveTicketPath(m.settings.TicketStore, result.Path)
	if result.Action == actionTranscript {
		return m, cmdTranscript(file, address, m.width)
	}
	if result.Action == actionWatch && address != "" {
		return m, cmdWatchAddress(address, result.TicketID)
	}
	result.Path = file
	if cmd, ok := answerActionCmd(m.worktreeRoot, m.settings, result); ok {
		return m, cmd
	}
	return m, cmdApplySuggestedAction(file, result.Action, func() tea.Msg { return queueActionAppliedMsg{} })
}

// cmdWatchAddress opens address's live native agent, else focuses its herdr
// pane. Liveness shells out to ps, so it is resolved inside the Cmd.
func cmdWatchAddress(address, ticketID string) tea.Cmd {
	epic, _, _ := tickets.SplitTrailerValue(address)
	watch, focus := cmdWatchAgent(address), cmdFocusPane(epic, ticketID)
	return func() tea.Msg {
		if nativeAgentLive(address) {
			return watch()
		}
		return focus()
	}
}

// queueActionAppliedMsg reports that cmdApplySuggestedAction's write finished
// successfully, so updateInner can trigger the same reload cmdLoadQueue uses.
type queueActionAppliedMsg struct{}
