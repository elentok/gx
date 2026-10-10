package tickets

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/viewmodel"
)

// WithServerState hands the tab the app shell's shared state; nil means no
// snapshot has arrived yet. The returned command starts the running spinner
// when the tab goes from idle to running.
func (m QueueModel) WithServerState(st *viewmodel.State) (tea.Model, tea.Cmd) {
	if st == nil || m.serverAPI == nil {
		return m, nil
	}
	m.shared = st
	if m.serverDown {
		return m, nil
	}
	return m.applyServerState()
}

// applyServerState rebuilds rows, queue order, claim times, budget, herdr state
// and queue mode from the shared state.
func (m QueueModel) applyServerState() (QueueModel, tea.Cmd) {
	st := m.shared
	m.serverClaimedAt = map[string]time.Time{}
	for _, t := range st.Tickets {
		if !t.ClaimedAt.IsZero() {
			m.serverClaimedAt[t.Address] = t.ClaimedAt
		}
	}
	m.herdrDown, m.serverBudget = st.HerdrUnavailable, st.Budget
	m.paused = st.Mode == server.ModePaused
	m.checked = make(map[string]bool, len(st.Queue))
	m.checkOrder = make(map[string]uint64, len(st.Queue))
	for i, addr := range st.Queue {
		m.checked[addr] = true
		m.checkOrder[addr] = uint64(i + 1)
	}
	m.applyEpics(epicsFromViewModel(*st, viewmodel.Scope{}))
	return m, m.syncServerRunState()
}

// applyEpics swaps the rows in and re-derives what hangs off them.
func (m *QueueModel) applyEpics(epics []gxtickets.Epic) {
	m.loaded = true
	m.epics = epics
	m.candidates = make(map[string]bool, len(m.checked))
	for path := range m.checked {
		m.candidates[path] = true
	}
	if m.search.HasQuery() {
		m.recomputeQueueSearchMatches()
	}
	m.clampSelected()
}

// OnPageActivated restarts the spinner loop, which only matters on screen.
func (m QueueModel) OnPageActivated() tea.Cmd {
	return func() tea.Msg { return queueSpinnerRestartMsg{} }
}

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
