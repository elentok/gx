package tickets

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
)

const serverModePaused = "paused"

// serverDoneMsg reports a finished server write: ok is the success toast, and a
// refusal or error becomes an error toast. mode is the queue mode the server
// reported, when the verb returns one.
type serverDoneMsg struct {
	ok      string
	problem string
	mode    string
}

// serverPauseStateMsg carries the budget the "p" key reads before deciding
// which of override, pause or resume to offer.
type serverPauseStateMsg struct {
	budget server.BudgetStatus
	err    error
}

// serverRemovedMsg reports finished dequeues: how many were removed and the
// first failure or refusal, if any.
type serverRemovedMsg struct {
	removed int
	problem string
}

type serverPauseAction int

const (
	serverActionPause serverPauseAction = iota
	serverActionResume
	serverActionOverride
)

// serverPauseActionFor is the three states of "p": a latched budget wins
// (override), else a paused queue resumes, else it pauses.
func serverPauseActionFor(b server.BudgetStatus, paused bool) serverPauseAction {
	switch {
	case b.BudgetPaused || b.HardLatched:
		return serverActionOverride
	case paused:
		return serverActionResume
	}
	return serverActionPause
}

func (m QueueModel) cmdServerRemove(addrs []string) tea.Cmd {
	api := m.serverAPI
	return func() tea.Msg {
		out := serverRemovedMsg{}
		for _, addr := range addrs {
			res, err := api.QueueRemove(context.Background(), addr)
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

// handleServerClearKey is "c"/"C" in server mode: dequeue the given tickets.
func (m QueueModel) handleServerClearKey(prompt string, addrs []string) (tea.Model, tea.Cmd) {
	m.confirm = m.confirm.Open(confirm.Options{Prompt: prompt, AcceptCmd: m.cmdServerRemove(addrs)})
	return m, nil
}

// handleServerDeleteKey is "x" in server mode. Delete is queue-only: the server
// removes the entry and its queued descendants, and refuses with ticket-live
// while an iteration runs (cancel is a status change, not a delete).
func (m QueueModel) handleServerDeleteKey() (tea.Model, tea.Cmd) {
	row, ok := m.selectedQueueRow()
	if !ok {
		return m, nil
	}
	prompt := fmt.Sprintf("Delete %s %s and every queued ticket it blocks?", row.ticket.DisplayNumber(), row.ticket.Title)
	m.confirm = m.confirm.Open(confirm.Options{Prompt: prompt, AcceptCmd: m.cmdServerRemove([]string{row.ticket.Path})})
	return m, nil
}

// proposedRemedyHeading is the ticket section the server writes when a
// high-authority recovery proposes a remedy (server/recovery.go).
const proposedRemedyHeading = "Proposed Remedy"

func hasPendingProposal(body string) bool {
	return schema.Section(body, proposedRemedyHeading) != ""
}

// handleServerApproveKey is "A" in server mode: run the selected ticket's
// pending recovery proposal. The server still refuses a stale or already-run
// proposal, and the refusal becomes the toast.
func (m QueueModel) handleServerApproveKey() (tea.Model, tea.Cmd) {
	row, ok := m.selectedQueueRow()
	if m.serverAPI == nil || !ok || !hasPendingProposal(row.ticket.Body) {
		return m, nil
	}
	api, addr := m.serverAPI, row.ticket.Path
	prompt := fmt.Sprintf("Run the recovery proposal for %s %s?", row.ticket.DisplayNumber(), row.ticket.Title)
	m.confirm = m.confirm.Open(confirm.Options{Prompt: prompt, AcceptCmd: func() tea.Msg {
		res, err := api.TicketApprove(context.Background(), addr)
		switch {
		case err != nil:
			return serverDoneMsg{problem: err.Error()}
		case res.Refused:
			return serverDoneMsg{problem: res.Message}
		}
		return serverDoneMsg{ok: "approved " + addr}
	}})
	return m, nil
}

// handleServerPauseKey is "p" in server mode: read the budget, then offer the
// matching action.
func (m QueueModel) handleServerPauseKey() (tea.Model, tea.Cmd) {
	api := m.serverAPI
	return m, func() tea.Msg {
		b, err := api.Budget(context.Background())
		return serverPauseStateMsg{budget: b, err: err}
	}
}

func (m QueueModel) openServerPauseConfirm(b server.BudgetStatus) QueueModel {
	api := m.serverAPI
	var prompt string
	var accept tea.Cmd
	switch serverPauseActionFor(b, m.paused) {
	case serverActionOverride:
		prompt = fmt.Sprintf("Budget limit reached ($%.2f spent). Override and resume?", b.Total)
		accept = func() tea.Msg {
			res, err := api.BudgetOverride(context.Background())
			switch {
			case err != nil:
				return serverDoneMsg{problem: err.Error()}
			case res.Refused:
				return serverDoneMsg{problem: res.Message}
			}
			return serverDoneMsg{ok: "budget pause overridden", mode: "running"}
		}
	case serverActionResume:
		prompt = "Resume the queue?"
		accept = cmdServerQueueMode("queue resumed", api.QueueResume)
	default:
		prompt = "Pause the queue?"
		accept = cmdServerQueueMode("queue paused", api.QueuePause)
	}
	m.confirm = m.confirm.Open(confirm.Options{Prompt: prompt, AcceptCmd: accept})
	return m
}

func cmdServerQueueMode(ok string, verb func(context.Context) (server.QueueResult, error)) tea.Cmd {
	return func() tea.Msg {
		res, err := verb(context.Background())
		switch {
		case err != nil:
			return serverDoneMsg{problem: err.Error()}
		case res.Refused:
			return serverDoneMsg{problem: res.Message}
		}
		return serverDoneMsg{ok: ok, mode: res.Mode}
	}
}

// updateServerKeys handles the Queue tab's server-key results; ok is false for
// any other msg.
func (m QueueModel) updateServerKeys(msg tea.Msg) (QueueModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case serverPauseStateMsg:
		if msg.err != nil {
			return m, notify.Error("budget: " + msg.err.Error()), true
		}
		return m.openServerPauseConfirm(msg.budget), nil, true
	case serverDoneMsg:
		if msg.problem != "" {
			return m, notify.Error("refused: " + msg.problem), true
		}
		if msg.mode != "" {
			m.paused = msg.mode == serverModePaused
		}
		return m, notify.Success(msg.ok), true
	case serverRemovedMsg:
		note := notify.Success(fmt.Sprintf("removed %d ticket(s) from the queue", msg.removed))
		if msg.problem != "" {
			note = notify.Error(fmt.Sprintf("removed %d, stopped at %s", msg.removed, msg.problem))
		}
		return m, tea.Batch(note, m.cmdLoadQueue()), true
	}
	return m, nil, false
}

// handleServerDrainKey is "D" in server mode: stop the server handing out new
// work; running iterations finish.
func (m Model) handleServerDrainKey() (tea.Model, tea.Cmd) {
	api := m.serverAPI
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    "Drain the queue? Running iterations finish; nothing new starts.",
		AcceptCmd: cmdServerQueueMode("queue draining", api.QueueDrain),
	})
	return m, nil
}
