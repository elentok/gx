package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/notify"
)

// Server-write verbs offered by the Context menu's Server group.
const (
	serverVerbPause  = "pause"
	serverVerbResume = "resume"
	serverVerbUnmute = "unmute"
	serverVerbCancel = "cancel"

	// serverPauseReason and serverStartOverReason are the one-line reasons the
	// menu records; the server requires one and the menu has no text input.
	serverPauseReason     = "paused from the TUI"
	serverStartOverReason = "started over from the TUI"
)

// serverDirectStatuses are the statuses a person sets directly, in the
// Context menu's Status group order: the rest (claimed, paused (answer), paused (repair)) are orchestration
// statuses only the server's verbs change.
var serverDirectStatuses = []schema.Status{schema.StatusOpen, schema.StatusDraft, schema.StatusDone}

// isRepairPause is a ticket paused for a fault or by a person, the only pauses
// that "enter" resumes: an answer pause needs the Answer action's own
// confirmation.
func isRepairPause(t tickets.Ticket) bool {
	switch schema.Status(t.Status) {
	case schema.StatusLegacyRepair:
		return true
	case schema.StatusPaused:
		return t.PauseReason.Class() == schema.PauseClassRepair || t.PauseReason == schema.PauseManual
	}
	return false
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
		repair := func(repairVerb string, req server.RepairRequest) {
			var rr server.RepairResult
			rr, err = api.Repair(ctx, repairVerb, req)
			res = server.QueueResult{Refused: rr.Refused, Message: rr.Message}
		}
		switch verb {
		case serverVerbPause:
			res, err = api.TicketPause(ctx, address, serverPauseReason)
		case serverVerbCancel:
			res, err = api.TicketCancel(ctx, address, true)
		case serverVerbUnmute:
			res, err = api.TicketUnmute(ctx, address)
		case server.ActionRetry:
			res, err = api.TicketRelaunch(ctx, address, false)
		case server.ActionApprove:
			res, err = api.TicketApprove(ctx, address)
		case server.ActionStashMerge:
			epic, _, _ := tickets.SplitTrailerValue(address)
			res, err = api.EpicStashMerge(ctx, epic)
		case server.ActionMerge:
			repair(verb, server.RepairRequest{Address: address})
		case server.ActionStartOver:
			repair(verb, server.RepairRequest{Address: address, Reason: serverStartOverReason})
		default:
			// resume, and the verbs that lift a pause through it.
			repair(serverVerbResume, server.RepairRequest{Address: address})
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

// handleServerResumeEnter is "enter" on a paused row in server mode: it runs
// the first verb of the ticket's detail.actions. handled is false for any other
// row so "enter" keeps its usual meaning.
func (m Model) handleServerResumeEnter() (cmd tea.Cmd, handled bool) {
	r, ok := m.selectedRow()
	if m.serverAPI == nil || !ok || r.isEpic() {
		return nil, false
	}
	ticket := m.epicAt(r).Tickets[r.ticketIdx]
	if !isRepairPause(ticket) {
		return nil, false
	}
	return cmdRunFirstAction(m.serverAPI, ticket.Path), true
}

// cmdRunFirstAction reads the ticket's detail and runs its first action; with
// none it does nothing.
func cmdRunFirstAction(api ServerAPI, address string) tea.Cmd {
	return func() tea.Msg {
		d, err := api.Detail(context.Background(), address)
		if err != nil {
			return notify.Error("detail: " + err.Error())()
		}
		if len(d.Actions) == 0 {
			return nil
		}
		return cmdServerWrite(api, d.Actions[0], address)()
	}
}
