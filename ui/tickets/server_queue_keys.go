package tickets

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

// serverDoneMsg reports a finished server write: ok is the success toast, and a
// refusal or error becomes an error toast. The new state arrives on the stream.
type serverDoneMsg struct {
	ok      string
	problem string
}

// serverPauseStateMsg carries the budget the "p" key reads before deciding
// which of override, pause or resume to offer.
type serverPauseStateMsg struct {
	budget wire.BudgetStatus
	err    error
}

// serverRemovedMsg reports finished dequeues: how many were removed and the
// first failure or refusal, if any.
type serverRemovedMsg struct {
	removed int
	problem string
}

const finishAndPausePrompt = "Finish and pause the queue? Running tickets finish; nothing new starts."

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

// serverKeys is the server-mode writes behind the Queue tab's keys: the
// sub-clients they call and the confirmations they open. Each method returns the
// confirm.Options (or command) for the tab to open, so it never touches the tab.
type serverKeys struct {
	queue   tabapi.QueueAPI
	admin   tabapi.AdminAPI
	tickets tabapi.TicketsAPI
	reads   tabapi.ReadsAPI
}

func (k serverKeys) cmdRemove(addrs []string) tea.Cmd {
	api := k.queue
	return func() tea.Msg {
		out := serverRemovedMsg{}
		for _, addr := range addrs {
			res, err := api.Remove(context.Background(), addr)
			switch {
			case err != nil:
				out.problem = addr + ": " + err.Error()
			case res.Refused:
				out.problem = addr + ": " + res.Message
			default:
				out.removed++
				continue
			}
			break
		}
		return out
	}
}

// clearConfirm is "c"/"C" in server mode: dequeue the given tickets.
func (k serverKeys) clearConfirm(prompt string, addrs []string) confirm.Options {
	return confirm.Options{Prompt: prompt, AcceptCmd: k.cmdRemove(addrs)}
}

// deleteConfirm is "x" in server mode. Delete is queue-only: the server
// removes the entry and its queued descendants, and refuses with ticket-live
// while an iteration runs (cancel is a status change, not a delete).
func (k serverKeys) deleteConfirm(t tickets.Ticket) confirm.Options {
	prompt := fmt.Sprintf("Delete %s %s and every queued ticket it blocks?", t.DisplayNumber(), t.Title)
	return confirm.Options{Prompt: prompt, AcceptCmd: k.cmdRemove([]string{t.Path})}
}

// handleServerApproveKey is "A" in server mode, and the Approve entry of the
// ticket menu: confirm the selected ticket's pending recovery proposal.
func (m QueueModel) handleServerApproveKey() (tea.Model, tea.Cmd) {
	if row, ok := m.selectedQueueRow(); ok {
		if opts, ok := m.srv.approveConfirm(row.ticket); ok {
			m.confirm = m.confirm.Open(opts)
		}
	}
	return m, nil
}

// approveConfirm is "A" in server mode: run the ticket's pending recovery
// proposal. The server still refuses a stale or already-run proposal, and the
// refusal becomes the toast. ok is false when there is nothing to approve.
func (k serverKeys) approveConfirm(t tickets.Ticket) (opts confirm.Options, ok bool) {
	if k.tickets == nil || !rowactions.HasPendingProposal(t.Body) {
		return confirm.Options{}, false
	}
	api, addr := k.tickets, t.Path
	prompt := fmt.Sprintf("Run the recovery proposal for %s %s?", t.DisplayNumber(), t.Title)
	return confirm.Options{Prompt: prompt, AcceptCmd: func() tea.Msg {
		res, err := api.Approve(context.Background(), addr)
		switch {
		case err != nil:
			return serverDoneMsg{problem: err.Error()}
		case res.Refused:
			return serverDoneMsg{problem: res.Message}
		}
		return serverDoneMsg{ok: "approved " + addr}
	}}, true
}

// cmdPause is "p" in server mode: read the budget, then offer the matching
// action (see pauseConfirm).
func (k serverKeys) cmdPause() tea.Cmd {
	api := k.reads
	return func() tea.Msg {
		b, err := api.Budget(context.Background())
		return serverPauseStateMsg{budget: b, err: err}
	}
}

func (k serverKeys) pauseConfirm(b wire.BudgetStatus, paused bool) confirm.Options {
	admin, queue := k.admin, k.queue
	var prompt string
	var accept tea.Cmd
	switch serverPauseActionFor(b, paused) {
	case serverActionOverride:
		prompt = fmt.Sprintf("Budget limit reached ($%.2f spent). Override and resume?", b.Total)
		accept = func() tea.Msg {
			res, err := admin.BudgetOverride(context.Background())
			switch {
			case err != nil:
				return serverDoneMsg{problem: err.Error()}
			case res.Refused:
				return serverDoneMsg{problem: res.Message}
			}
			return serverDoneMsg{ok: "budget pause overridden"}
		}
	case serverActionResume:
		prompt = "Resume the queue?"
		accept = cmdServerQueueMode("queue resumed", queue.Resume)
	default:
		prompt = finishAndPausePrompt
		accept = cmdServerQueueMode("queue paused", queue.Pause)
	}
	return confirm.Options{Prompt: prompt, AcceptCmd: accept}
}

func cmdServerQueueMode(ok string, verb func(context.Context) (wire.QueueResult, error)) tea.Cmd {
	return func() tea.Msg {
		res, err := verb(context.Background())
		switch {
		case err != nil:
			return serverDoneMsg{problem: err.Error()}
		case res.Refused:
			return serverDoneMsg{problem: res.Message}
		}
		return serverDoneMsg{ok: ok}
	}
}

// update handles the Queue tab's server-key results; ok is false for any other
// msg. A non-nil open is a confirmation for the tab to show.
func (k serverKeys) update(msg tea.Msg, paused bool) (cmd tea.Cmd, open *confirm.Options, ok bool) {
	switch msg := msg.(type) {
	case serverPauseStateMsg:
		if msg.err != nil {
			return notify.Error("budget: " + msg.err.Error()), nil, true
		}
		opts := k.pauseConfirm(msg.budget, paused)
		return nil, &opts, true
	case serverDoneMsg:
		if msg.problem != "" {
			return notify.Error("refused: " + msg.problem), nil, true
		}
		return notify.Success(msg.ok), nil, true
	case rowactions.ServerWriteMsg:
		return msg.Toast(), nil, true
	case rowactions.EpicQueuedMsg:
		return msg.Toast(), nil, true
	case serverRemovedMsg:
		note := notify.Success(fmt.Sprintf("removed %d ticket(s) from the queue", msg.removed))
		if msg.problem != "" {
			note = notify.Error(fmt.Sprintf("removed %d, stopped at %s", msg.removed, msg.problem))
		}
		return note, nil, true
	}
	return nil, nil, false
}

// handleServerFinishAndPauseKey is "D" in server mode, an alias of Finish and pause kept
// for one release: stop the server handing out new work; running tickets finish.
func (m Model) handleServerFinishAndPauseKey() (tea.Model, tea.Cmd) {
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    finishAndPausePrompt,
		AcceptCmd: cmdServerQueueMode("queue paused", m.queue.Pause),
	})
	return m, nil
}
