package queue

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// ServerDoneMsg reports a finished server write: OK is the success toast, and a
// refusal or error becomes an error toast. The new state arrives on the stream.
type ServerDoneMsg struct {
	OK      string
	Problem string
}

// serverPauseStateMsg carries the budget the "p" key reads before deciding
// which of override, pause or resume to offer.
type serverPauseStateMsg struct {
	budget wire.BudgetStatus
	err    error
}

// ServerRemovedMsg reports finished dequeues: how many were removed and the
// first failure or refusal, if any.
type ServerRemovedMsg struct {
	Removed int
	Problem string
}

const FinishAndPausePrompt = "Finish and pause the queue? Running tickets finish; nothing new starts."

type serverPauseAction int

const (
	serverActionPause serverPauseAction = iota
	serverActionResume
	serverActionOverride
)

// serverPauseActionFor is the three states of "p": a latched budget wins
// (override), else a paused queue resumes, else it pauses.
func serverPauseActionFor(b wire.BudgetStatus, paused bool) serverPauseAction {
	switch {
	case b.BudgetPaused || b.HardLatched:
		return serverActionOverride
	case paused:
		return serverActionResume
	}
	return serverActionPause
}

// Keys is the server-mode writes behind the Queue tab's keys: the
// sub-clients they call and the confirmations they open. Each method returns the
// confirm.Options (or command) for the tab to open, so it never touches the tab.
type Keys struct {
	Queue   tabapi.QueueAPI
	Admin   tabapi.AdminAPI
	Tickets tabapi.TicketsAPI
	Reads   tabapi.ReadsAPI
}

func (k Keys) CmdRemove(addrs []string) tea.Cmd {
	api := k.Queue
	return func() tea.Msg {
		out := ServerRemovedMsg{}
		for _, addr := range addrs {
			res, err := api.Remove(context.Background(), addr)
			switch {
			case err != nil:
				out.Problem = addr + ": " + err.Error()
			case res.Refused:
				out.Problem = addr + ": " + res.Message
			default:
				out.Removed++
				continue
			}
			break
		}
		return out
	}
}

// ClearConfirm is "c"/"C" in server mode: dequeue the given tickets.
func (k Keys) ClearConfirm(prompt string, addrs []string) confirm.Options {
	return confirm.Options{Prompt: prompt, AcceptCmd: k.CmdRemove(addrs)}
}

// DeleteConfirm is "x" in server mode. Delete is queue-only: the server
// removes the entry and its queued descendants, and refuses with ticket-live
// while an iteration runs (cancel is a status change, not a delete).
func (k Keys) DeleteConfirm(t tickets.Ticket) confirm.Options {
	prompt := fmt.Sprintf("Delete %s %s and every queued ticket it blocks?", t.DisplayNumber(), t.Title)
	return confirm.Options{Prompt: prompt, AcceptCmd: k.CmdRemove([]string{t.Path})}
}

// ApproveConfirm is "A" in server mode: run the ticket's pending recovery
// proposal. The server still refuses a stale or already-run proposal, and the
// refusal becomes the toast. ok is false when there is nothing to approve.
func (k Keys) ApproveConfirm(t tickets.Ticket) (opts confirm.Options, ok bool) {
	if k.Tickets == nil || !rowactions.HasPendingProposal(t.Body) {
		return confirm.Options{}, false
	}
	api, addr := k.Tickets, t.Path
	prompt := fmt.Sprintf("Run the recovery proposal for %s %s?", t.DisplayNumber(), t.Title)
	return confirm.Options{Prompt: prompt, AcceptCmd: func() tea.Msg {
		res, err := api.Approve(context.Background(), addr)
		switch {
		case err != nil:
			return ServerDoneMsg{Problem: err.Error()}
		case res.Refused:
			return ServerDoneMsg{Problem: res.Message}
		}
		return ServerDoneMsg{OK: "approved " + addr}
	}}, true
}

// CmdPause is "p" in server mode: read the budget, then offer the matching
// action (see pauseConfirm).
func (k Keys) CmdPause() tea.Cmd {
	api := k.Reads
	return func() tea.Msg {
		b, err := api.Budget(context.Background())
		return serverPauseStateMsg{budget: b, err: err}
	}
}

func (k Keys) pauseConfirm(b wire.BudgetStatus, paused bool) confirm.Options {
	admin, queue := k.Admin, k.Queue
	var prompt string
	var accept tea.Cmd
	switch serverPauseActionFor(b, paused) {
	case serverActionOverride:
		prompt = fmt.Sprintf("Budget limit reached ($%.2f spent). Override and resume?", b.Total)
		accept = func() tea.Msg {
			res, err := admin.BudgetOverride(context.Background())
			switch {
			case err != nil:
				return ServerDoneMsg{Problem: err.Error()}
			case res.Refused:
				return ServerDoneMsg{Problem: res.Message}
			}
			return ServerDoneMsg{OK: "budget pause overridden"}
		}
	case serverActionResume:
		prompt = "Resume the queue?"
		accept = CmdServerQueueMode("queue resumed", queue.Resume)
	default:
		prompt = FinishAndPausePrompt
		accept = CmdServerQueueMode("queue paused", queue.Pause)
	}
	return confirm.Options{Prompt: prompt, AcceptCmd: accept}
}

func CmdServerQueueMode(ok string, verb func(context.Context) (wire.QueueResult, error)) tea.Cmd {
	return func() tea.Msg {
		res, err := verb(context.Background())
		switch {
		case err != nil:
			return ServerDoneMsg{Problem: err.Error()}
		case res.Refused:
			return ServerDoneMsg{Problem: res.Message}
		}
		return ServerDoneMsg{OK: ok}
	}
}

// Update handles the Queue tab's server-key results; ok is false for any other
// msg. A non-nil open is a confirmation for the tab to show.
func (k Keys) Update(msg tea.Msg, paused bool) (cmd tea.Cmd, open *confirm.Options, ok bool) {
	switch msg := msg.(type) {
	case serverPauseStateMsg:
		if msg.err != nil {
			return notify.Error("budget: " + msg.err.Error()), nil, true
		}
		opts := k.pauseConfirm(msg.budget, paused)
		return nil, &opts, true
	case ServerDoneMsg:
		if msg.Problem != "" {
			return notify.Error("refused: " + msg.Problem), nil, true
		}
		return notify.Success(msg.OK), nil, true
	case rowactions.ServerWriteMsg:
		return msg.Toast(), nil, true
	case rowactions.EpicQueuedMsg:
		return msg.Toast(), nil, true
	case ServerRemovedMsg:
		note := notify.Success(fmt.Sprintf("removed %d ticket(s) from the queue", msg.Removed))
		if msg.Problem != "" {
			note = notify.Error(fmt.Sprintf("removed %d, stopped at %s", msg.Removed, msg.Problem))
		}
		return note, nil, true
	}
	return nil, nil, false
}
