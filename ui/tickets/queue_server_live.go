package tickets

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/viewmodel"
)

// WithServerState hands the tab the app shell's shared state and the link it
// was read over; nil st means no snapshot has arrived yet. The returned command
// starts the running spinner when the tab goes from idle to running.
func (m QueueModel) WithServerState(st *viewmodel.State, link ServerLink) (tea.Model, tea.Cmd) {
	m.link = link
	m.olderServer = st != nil && !m.serverDown() && st.OlderServer()
	if m.serverDown() || m.olderServer {
		// Whatever the queue showed is stale; it refills on reconnect.
		m.epics, m.summaries, m.finished = nil, nil, nil
		m.loaded = m.loaded || m.olderServer
		m.clampSelected()
		return m, nil
	}
	if st == nil {
		return m, nil
	}
	return m.applyServerState(st)
}

// applyServerState rebuilds rows, queue order, claim times, budget, herdr state
// and queue mode from st.
func (m QueueModel) applyServerState(st *viewmodel.State) (QueueModel, tea.Cmd) {
	m.summaries = make(map[string]server.EpicSummary, len(st.Epics))
	for _, sum := range st.Epics {
		m.summaries[sum.Address] = sum
	}
	hadFinished := len(m.finished) > 0
	m.finished = st.RecentlyFinished
	m.herdrDown, m.serverBudget = st.HerdrUnavailable, st.Budget
	m.paused = st.Mode == server.ModePaused
	m.checked = make(map[string]bool, len(st.Queue))
	m.checkOrder = make(map[string]uint64, len(st.Queue))
	for i, addr := range st.Queue {
		m.checked[addr] = true
		m.checkOrder[addr] = uint64(i + 1)
	}
	m.applyEpics(epicsFromViewModel(*st, viewmodel.Scope{}), true)
	cmd := m.syncServerRunState()
	if !hadFinished && len(m.finished) > 0 {
		// The first finished entry has no chain yet; a delivery only reaches the
		// tab that is on screen.
		cmd = tea.Batch(cmd, m.restartFinishedTick())
	}
	return m, cmd
}

// applyEpics swaps the rows in and re-derives what hangs off them. With
// wholeEpic, every ticket of an epic holding a queue item is a candidate: the
// server schedules the epic's whole frontier, so a ticket added after the epic
// was queued must show too.
func (m *QueueModel) applyEpics(epics []gxtickets.Epic, wholeEpic bool) {
	m.loaded = true
	m.epics = epics
	m.candidates = make(map[string]bool, len(m.checked))
	for path := range m.checked {
		m.candidates[path] = true
	}
	for _, epic := range epics {
		if !wholeEpic || !slices.ContainsFunc(epic.Tickets, func(t gxtickets.Ticket) bool { return m.checked[t.Path] }) {
			continue
		}
		for _, t := range epic.Tickets {
			m.candidates[t.Path] = true
		}
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
// epic summaries: an epic runs when the server says so, and its active tickets
// are the ones with a run.
// It fills runningEpics and live for the header, row spinners and timers, and
// returns the spinner tick when the tab goes from idle to running. A ticket's
// timer counts from the server's claim time.
func (m *QueueModel) syncServerRunState() tea.Cmd {
	wasRunning := len(m.runningEpics) > 0
	if m.runningEpics == nil {
		m.runningEpics = map[string]bool{}
	}
	live := map[string]map[string]liveTicketState{}
	running := map[string]bool{}
	for _, epic := range m.epics {
		sum := m.summaries[epic.Name]
		if sum.State != server.StateRunning {
			continue
		}
		active := make(map[string]time.Time, len(sum.Active))
		for _, a := range sum.Active {
			active[a.Address] = a.ClaimedAt
		}
		for _, t := range epic.Tickets {
			seen, ok := active[t.Path]
			if !ok {
				continue
			}
			if live[epic.Name] == nil {
				live[epic.Name] = map[string]liveTicketState{}
			}
			phase := livePhaseImplementing
			if t.Type == string(schema.TypeMerge) {
				phase = livePhaseMerging
			}
			live[epic.Name][t.Identifier] = liveTicketState{running: true, phase: phase, startedAt: seen}
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
