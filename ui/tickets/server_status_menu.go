package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/notify"
)

// Server-write verbs offered by the Context menu's Server group.
const (
	serverVerbPark     = "park"
	serverVerbUnpark   = "unpark"
	serverVerbUnmute   = "unmute"
	serverVerbCancel   = "cancel"
	serverVerbRelaunch = "relaunch"

	// serverParkReason is the one-line reason the menu records; the server
	// requires one and the menu has no text input.
	serverParkReason = "parked from the TUI"
)

// serverDirectStatuses are the statuses a person sets directly, in the
// Context menu's Status group order: the rest (claimed, needs-answer, needs-repair) are orchestration
// statuses only the server's verbs change.
var serverDirectStatuses = []schema.Status{schema.StatusOpen, schema.StatusDraft, schema.StatusDone}

func isParkedStatus(status string) bool {
	return schema.Status(status) == schema.StatusNeedsRepair
}

// serverWriteMsg reports a finished server verb: the refusal or error, if any.
type serverWriteMsg struct {
	verb    string
	problem string
}

// cmdServerWrite issues verb for address through the server. With no server
// it writes nothing: orchestration writes have no file fallback.
func cmdServerWrite(api ServerAPI, verb, address string) tea.Cmd {
	if api == nil {
		return notify.Warning(serverNotRunning)
	}
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
		case serverVerbUnmute:
			res, err = api.TicketUnmute(ctx, address)
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
	if m.serverAPI == nil || !ok || r.isEpic() {
		return nil, false
	}
	ticket := m.epicAt(r).Tickets[r.ticketIdx]
	if !isParkedStatus(ticket.Status) {
		return nil, false
	}
	return cmdServerWrite(m.serverAPI, serverVerbUnpark, ticket.Path), true
}
