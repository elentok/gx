package tickets

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/confirm"
)

// queueDetachedLiveMsg carries cmdCheckDetachedLive's result: this process is
// Detached (attachLockHeld is false) but epicNames still have total claimed/
// needs-attention tickets, alive of which have a confirmed-live herdr tab.
type queueDetachedLiveMsg struct {
	epicNames    []string
	total, alive int
}

// cmdCheckDetachedLive detects a Detached+Live queue: tickets left claimed/
// needs-attention by a gx process that exited without releasing the attach
// lock, some of which still have a live herdr pane. Returns nil (no dialog)
// whenever this process or a foreign process already holds the attach lock,
// or when nothing is claimed/needs-attention, or when the live-tab scan finds
// nothing actually alive.
func cmdCheckDetachedLive(worktreeRoot string) tea.Cmd {
	return func() tea.Msg {
		scratchDir := scratchDirFor(worktreeRoot)
		if attachLockHeld(scratchDir) {
			return nil
		}
		epics, err := tickets.Load(scratchDir)
		if err != nil {
			return nil
		}
		total := 0
		for _, epic := range epics {
			for _, t := range epic.Tickets {
				status := strings.ToLower(strings.TrimSpace(t.Status))
				if status == "claimed" || status == "needs-attention" {
					total++
				}
			}
		}
		if total == 0 {
			return nil
		}
		signals, err := ralphloop.ScanForReattachable(reattachFindWorkspace, reattachTabList, epics)
		if err != nil || len(signals) == 0 {
			return nil
		}
		names := make(map[string]bool, len(signals))
		for _, s := range signals {
			names[s.EpicName] = true
		}
		epicNames := make([]string, 0, len(names))
		for name := range names {
			epicNames = append(epicNames, name)
		}
		sort.Strings(epicNames)
		return queueDetachedLiveMsg{epicNames: epicNames, total: total, alive: len(signals)}
	}
}

// detachedLiveConfirmedMsg carries the Detached+Live confirmation modal's
// acceptance (see handleDetachedLiveDetected) — mirrors queue_delete.go's
// cascadeDeleteConfirmedMsg pattern.
type detachedLiveConfirmedMsg struct {
	epicNames []string
}

// cmdConfirmDetachedLive returns the tea.Cmd run when the Detached+Live
// confirmation modal is accepted (see confirm.Options.AcceptCmd).
func cmdConfirmDetachedLive(epicNames []string) tea.Cmd {
	return func() tea.Msg {
		return detachedLiveConfirmedMsg{epicNames: epicNames}
	}
}

// planForFullEpic builds a dynamic whole-epic plan (independent of the
// checked set) for resuming a Detached+Live epic: every ticket in epic,
// dynamic so startAvailableEpics launches with an empty RunOptions.TicketIDs
// and reconcile naturally picks up whatever's still claimed/needs-attention.
func planForFullEpic(epic tickets.Epic) checkedEpicPlan {
	ticketIDs := make([]string, 0, len(epic.Tickets))
	done := 0
	for _, idx := range sortedTicketIndexes(epic) {
		ticket := epic.Tickets[idx]
		ticketIDs = append(ticketIDs, ticket.Identifier)
		if epic.RenderedStatus(ticket) == tickets.StatusDone {
			done++
		}
	}
	return checkedEpicPlan{epic: epic, ticketIDs: ticketIDs, dynamic: true, done: done}
}

// handleDetachedLiveDetected opens the combined confirmation dialog for
// cmdCheckDetachedLive's result, unless a dialog is already open (e.g. the
// cascade-delete confirm), which must not be clobbered.
func (m QueueModel) handleDetachedLiveDetected(msg queueDetachedLiveMsg) (tea.Model, tea.Cmd) {
	if m.confirm.IsOpen {
		return m, nil
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    fmt.Sprintf("Found a detached live queue with %d ticket(s), %d actively running. Reattach?", msg.total, msg.alive),
		AcceptCmd: cmdConfirmDetachedLive(msg.epicNames),
	})
	return m, nil
}

// handleDetachedLiveConfirmed queues a dynamic full-epic plan for every named
// epic and starts them via the normal pending-epic path — see this ticket's
// "Why resuming needs a real Run()" note: acquiring the attach lock alone
// doesn't re-supervise a live herdr pane, only startAvailableEpics ->
// cmdStartImplement -> loopRegistry.tryStart does, as a side effect of
// actually starting a Run().
func (m QueueModel) handleDetachedLiveConfirmed(msg detachedLiveConfirmedMsg) (tea.Model, tea.Cmd) {
	names := make(map[string]bool, len(msg.epicNames))
	for _, name := range msg.epicNames {
		names[name] = true
	}
	for _, epic := range m.epics {
		if names[epic.Name] {
			m.pendingEpics = append(m.pendingEpics, planForFullEpic(epic))
		}
	}
	if m.runningAgent == "" {
		m.runningAgent = ralphloop.AgentClaude
	}
	return m, m.startAvailableEpics()
}
