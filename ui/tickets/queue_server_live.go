package tickets

import (
	tea "charm.land/bubbletea/v2"

	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// syncServerRunState derives the Queue tab's running state from the server's
// ticket snapshot: no run happens in-process, so a claimed ticket is the
// only sign that an epic runs.
// It fills runningEpics and live for the header, row spinners and timers, and
// returns the spinner tick when the tab
// goes from idle to running. A ticket's timer counts from the server's claim
// time; a claimed ticket the server has no run for is not counted as running.
func (m *QueueModel) syncServerRunState() tea.Cmd {
	wasRunning := len(m.runningEpics) > 0
	if m.runningEpics == nil {
		m.runningEpics = map[string]bool{}
	}
	live := map[string]map[string]liveTicketState{}
	running := map[string]bool{}
	for _, epic := range m.epics {
		for _, t := range epic.Tickets {
			if epic.RenderedStatus(t) != gxtickets.StatusClaimed {
				continue
			}
			// No claim time means the server holds no run for it: a person
			// claimed it (a grilling session, say), so nothing is implementing.
			seen, ok := m.serverClaimedAt[t.Path]
			if !ok || seen.IsZero() {
				continue
			}
			if live[epic.Name] == nil {
				live[epic.Name] = map[string]liveTicketState{}
			}
			live[epic.Name][t.Identifier] = liveTicketState{running: true, phase: livePhaseImplementing, startedAt: seen}
			running[epic.Name] = true
		}
		markConflictResolution(epic, live[epic.Name])
	}
	m.live = live
	m.runningEpics = running
	if len(running) > 0 && !wasRunning {
		return m.implementSpinner.Tick
	}
	return nil
}

// markConflictResolution shows a claimed conflict-resolution child as resolving
// conflicts and its parent as waiting on it. The child runs inside the
// parent's land, so the server holds no run (and no claim time) of its own for
// it; its timer stays blank rather than borrowing the parent's. The child's raw
// status is checked: its rendered one reads blocked while the parent is not done.
func markConflictResolution(epic gxtickets.Epic, live map[string]liveTicketState) {
	for _, t := range epic.Tickets {
		if t.Type != string(schema.TypeConflictResolution) || t.Parent == nil ||
			t.Status != string(schema.StatusClaimed) {
			continue
		}
		parent, ok := live[*t.Parent]
		if !ok {
			continue
		}
		parent.phase, parent.waitingOn = livePhaseResolvingConflicts, t.DisplayNumber()
		live[*t.Parent] = parent
		live[t.Identifier] = liveTicketState{running: true, phase: livePhaseResolvingConflicts}
	}
}
