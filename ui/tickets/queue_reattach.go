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
// needs-repair tickets, alive of which have a confirmed-live herdr tab.
type queueDetachedLiveMsg struct {
	epicNames    []string
	total, alive int
}

// cmdCheckDetachedLive detects a Detached+Live queue: tickets left claimed/
// needs-repair by a gx process that exited without releasing the attach
// lock, some of which still have a live herdr pane. Returns nil (no dialog)
// whenever this process or a foreign process already holds the attach lock,
// or when nothing is claimed/needs-repair, or when the live-tab scan finds
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
				if status == "claimed" || status == "needs-repair" {
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
// and reconcile naturally picks up whatever's still claimed/needs-repair.
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

// queueStrandedPendingMsg names epics that were checked/queued in
// queue-state.json (durable Items) before this gx process started, have no
// live registry run and no claimed/needs-repair ticket (that combination
// belongs to queueDetachedLiveMsg's confirmation instead), and so silently
// fell out of MaxConcurrentEpics's auto-promotion queue — see
// requeueMaybeStrandedEpics's doc comment for why this can't be resolved
// without asking: "checked, not running" is indistinguishable on disk from
// "checked, Enter not pressed yet."
type queueStrandedPendingMsg struct {
	epicNames []string
}

// cmdCheckStrandedPending scans this model's own checked selection (m.checked
// — durable via the Queue tab's queueStore, see requeueMaybeStrandedEpics) for
// epics eligible to be re-promoted. Called once, from queueEpicsLoadedMsg's
// first load only (see its call site's comment) — every check afterward in
// this process's lifetime reflects the user's own checking, not a restart, so
// re-running the scan on every tab activation would nag mid-selection.
func (m QueueModel) cmdCheckStrandedPending() tea.Cmd {
	epics, checked, pendingEpics, runningEpics := m.epics, m.checked, m.pendingEpics, m.runningEpics
	return func() tea.Msg {
		names := requeueMaybeStrandedEpics(epics, checked, pendingEpics, runningEpics, byNameSnapshot())
		if len(names) == 0 {
			return nil
		}
		return queueStrandedPendingMsg{epicNames: names}
	}
}

// requeueMaybeStrandedEpics finds every epic with at least one checked,
// not-yet-done ticket that isn't already running or pending, and has no
// claimed/needs-repair ticket of its own (that shape is
// queueDetachedLiveMsg's — a live herdr pane may still be driving it, so it
// gets an explicit "reattach?" instead of silently folding in here).
//
// This can't tell "checked, Enter never pressed" apart from "checked, Enter
// was pressed, epic was waiting its turn in pendingEpics, then the gx process
// restarted before its slot freed" — both leave identical durable state
// (an Items entry, status "pending", no registry run). So this only reports
// candidates; the caller must still confirm with the user before touching
// m.pendingEpics (see handleStrandedPendingConfirmed), the same way
// queueDetachedLiveMsg never resumes without asking.
func requeueMaybeStrandedEpics(epics []tickets.Epic, checked map[string]bool, pendingEpics []checkedEpicPlan, runningEpics map[string]bool, byName map[string]RunSnapshot) []string {
	alreadyPending := make(map[string]bool, len(pendingEpics))
	for _, plan := range pendingEpics {
		alreadyPending[plan.epic.Name] = true
	}
	var names []string
	for _, epic := range epics {
		if runningEpics[epic.Name] || alreadyPending[epic.Name] {
			continue
		}
		if _, running := byName[epic.Name]; running {
			continue
		}
		if epicHasInFlightTicket(epic) {
			continue // claimed/needs-repair — queueDetachedLiveMsg owns this case
		}
		hasChecked, allDone := false, true
		for _, ticket := range epic.Tickets {
			if !checked[ticket.Path] {
				continue
			}
			hasChecked = true
			if epic.RenderedStatus(ticket) != tickets.StatusDone {
				allDone = false
			}
		}
		if hasChecked && !allDone {
			names = append(names, epic.Name)
		}
	}
	sort.Strings(names)
	return names
}

// epicHasInFlightTicket reports whether epic has any ticket a live herdr
// session might still be driving (claimed or needs-repair) — the same
// condition cmdCheckDetachedLive scans for.
func epicHasInFlightTicket(epic tickets.Epic) bool {
	for _, ticket := range epic.Tickets {
		switch epic.RenderedStatus(ticket) {
		case tickets.StatusClaimed, tickets.StatusNeedsRepair:
			return true
		}
	}
	return false
}

// handleStrandedPendingDetected opens the confirmation dialog for
// cmdCheckStrandedPending's result, unless a dialog is already open.
func (m QueueModel) handleStrandedPendingDetected(msg queueStrandedPendingMsg) (tea.Model, tea.Cmd) {
	if m.confirm.IsOpen {
		return m, nil
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt: fmt.Sprintf(
			"Found %d epic(s) checked/queued before this gx process (re)started, never claimed. Resume?",
			len(msg.epicNames),
		),
		AcceptCmd: cmdConfirmStrandedPending(msg.epicNames),
	})
	return m, nil
}

type strandedPendingConfirmedMsg struct {
	epicNames []string
}

func cmdConfirmStrandedPending(epicNames []string) tea.Cmd {
	return func() tea.Msg {
		return strandedPendingConfirmedMsg{epicNames: epicNames}
	}
}

// handleStrandedPendingConfirmed re-derives each named epic's plan from the
// checked selection (not a full-epic dynamic plan like
// handleDetachedLiveConfirmed's — only what was actually checked should
// resume) and queues it the normal way.
func (m QueueModel) handleStrandedPendingConfirmed(msg strandedPendingConfirmedMsg) (tea.Model, tea.Cmd) {
	names := make(map[string]bool, len(msg.epicNames))
	for _, name := range msg.epicNames {
		names[name] = true
	}
	for _, plan := range m.checkedEpicPlans() {
		if names[plan.epic.Name] {
			m.pendingEpics = append(m.pendingEpics, plan)
		}
	}
	if m.runningAgent == "" {
		m.runningAgent = ralphloop.AgentClaude
	}
	return m, m.startAvailableEpics()
}
