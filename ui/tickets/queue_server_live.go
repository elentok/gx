package tickets

import (
	"time"

	tea "charm.land/bubbletea/v2"

	gxtickets "github.com/elentok/gx/tickets"
)

// syncServerRunState derives the Queue tab's running state from the server's
// ticket snapshot: in server mode no in-process run registers with
// ralphLoopRegistry, so a claimed ticket is the only sign that an epic runs.
// It fills runningEpics and live so the header, row spinners and timers behave
// as they do for an in-process run, and returns the spinner tick when the tab
// goes from idle to running. A ticket's timer counts from the first load that
// saw it claimed: the snapshot carries no claim time.
func (m *QueueModel) syncServerRunState() tea.Cmd {
	wasRunning := len(m.runningEpics) > 0
	if m.runningEpics == nil {
		m.runningEpics = map[string]bool{}
	}
	if m.serverClaimSeen == nil {
		m.serverClaimSeen = map[string]time.Time{}
	}
	live := map[string]map[string]liveTicketState{}
	running := map[string]bool{}
	for _, epic := range m.epics {
		for _, t := range epic.Tickets {
			if epic.RenderedStatus(t) != gxtickets.StatusClaimed {
				delete(m.serverClaimSeen, t.Path)
				continue
			}
			seen, ok := m.serverClaimSeen[t.Path]
			if !ok {
				seen = time.Now()
				m.serverClaimSeen[t.Path] = seen
			}
			if live[epic.Name] == nil {
				live[epic.Name] = map[string]liveTicketState{}
			}
			live[epic.Name][t.Identifier] = liveTicketState{running: true, phase: livePhaseImplementing, startedAt: seen}
			running[epic.Name] = true
		}
	}
	m.live = live
	m.runningEpics = running
	if len(running) > 0 {
		if m.executionStartedAt.IsZero() {
			m.executionStartedAt = time.Now()
		}
		m.executionCompletedAt = time.Time{}
		if !wasRunning {
			return m.implementSpinner.Tick
		}
	}
	return nil
}
