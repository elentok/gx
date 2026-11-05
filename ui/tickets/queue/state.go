package queue

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/viewmodel"
)

// WithServerState hands the tab the app shell's shared state and the link it
// was read over; nil st means no snapshot has arrived yet. The returned command
// starts the running spinner when the tab goes from idle to running.
func (m QueueModel) WithServerState(st *viewmodel.State, link tabapi.ServerLink) (tea.Model, tea.Cmd) {
	m.server = m.server.WithLink(st, link)
	if m.server.Down() || m.server.OlderServer {
		// Whatever the queue showed is stale; it refills on reconnect.
		m.rows.Epics, m.server.Summaries, m.server.Finished = nil, nil, nil
		m.loaded = m.loaded || m.server.OlderServer
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
	hadFinished := len(m.server.Finished) > 0
	m.server = m.server.ApplyState(st)
	m.rows.Checked = make(map[string]bool, len(st.Queue))
	m.rows.CheckOrder = make(map[string]uint64, len(st.Queue))
	for i, addr := range st.Queue {
		m.rows.Checked[addr] = true
		m.rows.CheckOrder[addr] = uint64(i + 1)
	}
	m.applyEpics(tabapi.EpicsFromViewModel(*st, viewmodel.Scope{}), true)
	var cmd tea.Cmd
	m.server, cmd = m.server.SyncRunState(m.rows.Epics, m.implementSpinner)
	if !hadFinished && len(m.server.Finished) > 0 {
		// The first finished entry has no chain yet; a delivery only reaches the
		// tab that is on screen.
		var tick tea.Cmd
		m.server, tick = m.server.RestartFinishedTick()
		cmd = tea.Batch(cmd, tick)
	}
	return m, cmd
}

// applyEpics swaps the rows in and re-derives what hangs off them. With
// wholeEpic, every ticket of an epic holding a queue item is a candidate: the
// server schedules the epic's whole frontier, so a ticket added after the epic
// was queued must show too. The item is a ticket address or, from the web UI,
// the epic's own address.
func (m *QueueModel) applyEpics(epics []gxtickets.Epic, wholeEpic bool) {
	m.loaded = true
	m.rows.Epics = epics
	m.rows.Candidates = make(map[string]bool, len(m.rows.Checked))
	for path := range m.rows.Checked {
		m.rows.Candidates[path] = true
	}
	for _, epic := range epics {
		queued := m.rows.Checked[epic.Name] || slices.ContainsFunc(epic.Tickets, func(t gxtickets.Ticket) bool { return m.rows.Checked[t.Path] })
		if !wholeEpic || !queued {
			continue
		}
		for _, t := range epic.Tickets {
			m.rows.Candidates[t.Path] = true
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
