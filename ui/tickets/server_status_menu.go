package tickets

import (
	"context"
	"github.com/elentok/gx/server/wire"
	"slices"

	tea "charm.land/bubbletea/v2"

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

// isWebOnlyVerb is a detail.actions verb the TUI cannot run: it needs a reply or
// a base the TUI has no input for, or the server has no repair verb for it. The
// menu still lists them; picking one points the person to the web.
func isWebOnlyVerb(verb string) bool {
	switch verb {
	case wire.ActionAnswer, wire.ActionChooseBase, wire.ActionReopenAndRetry:
		return true
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
func cmdServerWrite(api ServerAPI, tix TicketsAPI, verb, address string) tea.Cmd {
	if api == nil || tix == nil {
		return notify.Warning(serverNotRunning)
	}
	if isWebOnlyVerb(verb) {
		return notify.Warning(verb + " needs the web UI")
	}
	return func() tea.Msg {
		ctx := context.Background()
		var (
			res wire.QueueResult
			err error
		)
		repair := func(repairVerb string, req wire.RepairRequest) {
			var rr wire.RepairResult
			rr, err = api.Repair(ctx, repairVerb, req)
			res = wire.QueueResult{Refused: rr.Refused, Message: rr.Message}
		}
		switch verb {
		case serverVerbPause:
			res, err = tix.Pause(ctx, address, serverPauseReason)
		case serverVerbCancel:
			res, err = tix.Cancel(ctx, address, true)
		case serverVerbUnmute:
			res, err = tix.Unmute(ctx, address)
		case wire.ActionRetry:
			res, err = tix.Relaunch(ctx, address, false)
		case wire.ActionApprove:
			res, err = tix.Approve(ctx, address)
		case wire.ActionStashMerge:
			epic, _, _ := tickets.SplitTrailerValue(address)
			res, err = tix.EpicStashMerge(ctx, epic)
		case wire.ActionMerge:
			repair(verb, wire.RepairRequest{Address: address})
		case wire.ActionStartOver:
			repair(verb, wire.RepairRequest{Address: address, Reason: serverStartOverReason})
		case wire.ActionResume:
			repair(serverVerbResume, wire.RepairRequest{Address: address})
		default:
			return notify.Warning("unknown action " + verb)()
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
	return cmdRunFirstAction(m.serverAPI, m.tickets, ticket.Path), true
}

// cmdRunFirstAction reads the ticket's detail and runs its first action; with
// none it does nothing.
func cmdRunFirstAction(api ServerAPI, tix TicketsAPI, address string) tea.Cmd {
	return func() tea.Msg {
		d, err := api.Detail(context.Background(), address)
		if err != nil {
			return notify.Error("detail: " + err.Error())()
		}
		i := slices.IndexFunc(d.Actions, func(a string) bool { return !isWebOnlyVerb(a) })
		if i < 0 {
			return nil
		}
		return cmdServerWrite(api, tix, d.Actions[i], address)()
	}
}
