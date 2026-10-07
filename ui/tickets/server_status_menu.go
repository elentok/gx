package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

// Server-write verbs offered by the "s" menu. In server mode a ticket row only
// carries its address (no file path), so the orchestration writes — the ones
// the server owns — are the whole menu; the direct file writes (open/draft)
// stay on the disk-backed menu.
const (
	serverVerbPark     = "park"
	serverVerbUnpark   = "unpark"
	serverVerbCancel   = "cancel"
	serverVerbRelaunch = "relaunch"

	// serverParkReason is the one-line reason the menu records; the server
	// requires one and the menu has no text input.
	serverParkReason = "parked from the TUI"
)

// serverMenuValuePrefix tells handleStatusMenuKey a menu value is a server verb
// rather than a schema.Status.
const serverMenuValuePrefix = "server:"

func isParkedStatus(status string) bool {
	return schema.Status(status) == schema.StatusNeedsRepair
}

// newServerStatusMenu lists the server actions that apply to ticket's state:
// a parked ticket unparks, any other unfinished one parks; both can be
// cancelled or relaunched. A finished ticket has none.
func newServerStatusMenu(ticket tickets.Ticket, rendered tickets.RenderedStatus) components.MenuState {
	if rendered.Terminal() {
		return components.MenuState{}
	}
	verbs := []string{serverVerbPark}
	if isParkedStatus(ticket.Status) {
		verbs = []string{serverVerbUnpark}
	}
	verbs = append(verbs, serverVerbCancel, serverVerbRelaunch)
	items := make([]components.MenuItem, len(verbs))
	for i, v := range verbs {
		items[i] = components.MenuItem{Label: v, Value: serverMenuValuePrefix + v}
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
