package tickets

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

// Server-write verbs offered by the "s" menu. In server mode the menu also
// offers serverDirectStatuses, written straight to the ticket's file.
const (
	serverVerbPark     = "park"
	serverVerbUnpark   = "unpark"
	serverVerbCancel   = "cancel"
	serverVerbRelaunch = "relaunch"

	// serverParkReason is the one-line reason the menu records; the server
	// requires one and the menu has no text input.
	serverParkReason = "parked from the TUI"
)

// serverDirectStatuses are the statuses a person sets directly in server
// mode: the rest (claimed, needs-answer, needs-repair) are orchestration
// statuses only the server's verbs change.
var serverDirectStatuses = []schema.Status{schema.StatusOpen, schema.StatusDraft, schema.StatusDone}

// serverMenuValuePrefix tells handleStatusMenuKey a menu value is a server verb
// rather than a schema.Status.
const serverMenuValuePrefix = "server:"

func isParkedStatus(status string) bool {
	return schema.Status(status) == schema.StatusNeedsRepair
}

// newServerStatusMenu lists, first, the direct statuses other than ticket's
// own (only when its file is known), then the server actions that apply to
// its state: a parked ticket unparks, any other unfinished one parks; both
// can be cancelled or relaunched. A finished ticket has no server actions.
func newServerStatusMenu(ticket tickets.Ticket, rendered tickets.RenderedStatus) components.MenuState {
	var items []components.MenuItem
	if ticket.File != "" {
		current := schema.Status(strings.ToLower(strings.TrimSpace(ticket.Status)))
		for _, status := range serverDirectStatuses {
			if status != current {
				items = append(items, components.MenuItem{Label: statusMenuLabels[status], Value: string(status)})
			}
		}
	}
	if rendered.Terminal() {
		return components.MenuState{Items: items}
	}
	verbs := []string{serverVerbPark}
	if isParkedStatus(ticket.Status) {
		verbs = []string{serverVerbUnpark}
	}
	verbs = append(verbs, serverVerbCancel, serverVerbRelaunch)
	for _, v := range verbs {
		items = append(items, components.MenuItem{Label: v, Value: serverMenuValuePrefix + v})
	}
	return components.MenuState{Items: items}
}

// serverWriteMsg reports a finished server verb: the refusal or error, if any.
type serverWriteMsg struct {
	verb    string
	problem string
}

// cmdServerWrite issues verb for address through the server.
func (m Model) cmdServerWrite(verb, address string) tea.Cmd {
	api := m.serverAPI
	return func() tea.Msg {
		ctx := context.Background()
		var (
			res server.QueueResult
			err error
		)
		switch verb {
		case serverVerbPark:
			res, err = api.TicketPark(ctx, address, serverParkReason)
		case serverVerbCancel:
			res, err = api.TicketCancel(ctx, address, true)
		case serverVerbRelaunch:
			res, err = api.TicketRelaunch(ctx, address)
		case serverVerbUnpark:
			var rr server.RepairResult
			rr, err = api.Repair(ctx, serverVerbUnpark, server.RepairRequest{Address: address})
			res = server.QueueResult{Refused: rr.Refused, Message: rr.Message}
		}
		switch {
		case err != nil:
			return serverWriteMsg{verb: verb, problem: err.Error()}
		case res.Refused:
			return serverWriteMsg{verb: verb, problem: res.Message}
		}
		return serverWriteMsg{verb: verb}
	}
}

func (msg serverWriteMsg) toast() tea.Cmd {
	if msg.problem != "" {
		return notify.Error(msg.verb + " refused: " + msg.problem)
	}
	return notify.Success(msg.verb + " done")
}

// handleServerUnparkEnter is "enter" on a parked row in server mode: it issues
// unpark. handled is false for any other row so "enter" keeps its usual meaning.
func (m Model) handleServerUnparkEnter() (cmd tea.Cmd, handled bool) {
	r, ok := m.selectedRow()
	if !m.serverMode() || !ok || r.isEpic() {
		return nil, false
	}
	ticket := m.epicAt(r).Tickets[r.ticketIdx]
	if !isParkedStatus(ticket.Status) {
		return nil, false
	}
	return m.cmdServerWrite(serverVerbUnpark, ticket.Path), true
}
