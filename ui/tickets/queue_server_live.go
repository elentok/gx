package tickets

import (
	"context"
	"slices"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/viewmodel"
)

// queueServer is what the Queue tab knows about the server: the delivered
// snapshot's derived state, the link it was read over and the clients that
// reach it. It is passed by value; methods return the next value.
type queueServer struct {
	// live is epicName -> ticket identifier -> running state, derived from the
	// delivered state (syncRunState), so the Queue tab's rows render the
	// running spinner+phase presentation (rowfmt.RenderLiveTicketRow).
	live map[string]map[string]rowfmt.LiveTicketState
	// summaries is the server's summary of each queued epic by epic name.
	summaries    map[string]wire.EpicSummary
	runningEpics map[string]bool
	// harnessLabels is the snapshot's row label by ticket address.
	harnessLabels map[string]string
	// waitingLabels is the Queue agent rows' check-lock label by ticket address.
	waitingLabels map[string]string
	// codexRows marks the snapshot tickets whose label says codex, by address.
	codexRows map[string]bool
	// herdrDown says the server cannot launch agents; herdrText names the
	// tickets waiting on herdr (empty falls back to the generic banner).
	herdrDown bool
	herdrText string
	// finished is the snapshot's Recently finished list. finishedGen numbers the
	// refresh tick chain: each page activation starts a new chain and the older
	// chain's pending tick is ignored.
	finished    []wire.FinishedEpic
	finishedGen int
	// serverBudget is the last snapshot's budget: the header's spend and the
	// root rows' cost.
	serverBudget wire.BudgetStatus
	paused       bool
	// serverAPI/serverStart/link back the server-down banner. link is only ever
	// set by WithServerState; a tab with no client is delivered ServerLinkDown,
	// so it shows the banner too.
	serverAPI   tabapi.ServerAPI
	serverStart func(context.Context) error
	link        tabapi.ServerLink
	// olderServer: see Model.olderServer.
	olderServer bool
}

// down reports the down banner state: the link the shell last delivered.
func (s queueServer) down() bool { return s.link == tabapi.ServerLinkDown }

// withLink records the link and whether the server predates the snapshot.
func (s queueServer) withLink(st *viewmodel.State, link tabapi.ServerLink) queueServer {
	s.link = link
	s.olderServer = st != nil && !s.down() && st.OlderServer()
	return s
}

// applyState derives the server-owned fields from st.
func (s queueServer) applyState(st *viewmodel.State) queueServer {
	s.summaries = make(map[string]wire.EpicSummary, len(st.Epics))
	for _, sum := range st.Epics {
		s.summaries[sum.Address] = sum
	}
	s.harnessLabels = make(map[string]string, len(st.Tickets))
	s.codexRows = make(map[string]bool)
	for _, t := range st.Tickets {
		s.codexRows[t.Address] = t.Harness == "codex"
		if l := ui.QueueHarnessLabel(t.Harness, t.Tier, t.Model, t.Effort, t.HarnessNote); l != "" {
			s.harnessLabels[t.Address] = l
		}
	}
	s.waitingLabels = make(map[string]string)
	for _, sess := range st.Sessions {
		if sess.Queue && sess.WaitingLabel != "" {
			s.waitingLabels[sess.Ticket] = sess.WaitingLabel
		}
	}
	s.finished = st.RecentlyFinished
	s.herdrDown, s.herdrText, s.serverBudget = st.HerdrUnavailable, st.HerdrText, st.Budget
	s.paused = st.Mode == wire.QueueModePaused
	return s
}

// restartFinishedTick starts a new refresh chain, orphaning the old one, or
// returns a nil command when there is no finished entry to refresh.
func (s queueServer) restartFinishedTick() (queueServer, tea.Cmd) {
	s.finishedGen++
	if len(s.finished) == 0 {
		return s, nil
	}
	return s, finishedTick(s.finishedGen)
}

// updateDown handles the server-start messages; ok is false for any other msg.
func (s queueServer) updateDown(msg tea.Msg) (cmd tea.Cmd, ok bool) {
	switch msg := msg.(type) {
	case queueServerStartConfirm:
		start := s.serverStart
		return func() tea.Msg { return queueServerStartedMsg{err: start(context.Background())} }, true
	case queueServerStartedMsg:
		if msg.err != nil {
			return notify.Error("start server: " + msg.err.Error()), true
		}
		return cmdProbeRequested, true
	}
	return nil, false
}

// startConfirm asks before starting the server; ok is false when the server is
// up or the tab has no way to start it.
func (s queueServer) startConfirm() (confirm.Options, bool) {
	if !s.down() || s.serverStart == nil {
		return confirm.Options{}, false
	}
	return confirm.Options{
		Prompt:    "server is down — start it now?",
		AcceptCmd: func() tea.Msg { return queueServerStartConfirm{} },
	}, true
}

// syncRunState derives the Queue tab's running state from the server's epic
// summaries: an epic runs when the server says so, and its active tickets are
// the ones with a run.
// It fills runningEpics and live for the header, row spinners and timers, and
// returns the spinner tick when the tab goes from idle to running. A ticket's
// timer counts from the server's claim time.
func (s queueServer) syncRunState(epics []gxtickets.Epic, sp spinner.Model) (queueServer, tea.Cmd) {
	wasRunning := len(s.runningEpics) > 0
	live := map[string]map[string]rowfmt.LiveTicketState{}
	running := map[string]bool{}
	for _, epic := range epics {
		sum := s.summaries[epic.Name]
		if sum.State != wire.EpicStateRunning {
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
				live[epic.Name] = map[string]rowfmt.LiveTicketState{}
			}
			phase := rowfmt.LivePhaseImplementing
			if t.Type == string(schema.TypeMerge) {
				phase = rowfmt.LivePhaseMerging
			}
			live[epic.Name][t.Identifier] = rowfmt.LiveTicketState{Running: true, Phase: phase, StartedAt: seen}
			running[epic.Name] = true
		}
		markConflictResolution(epic, live[epic.Name])
	}
	s.live = live
	s.runningEpics = running
	if len(running) > 0 && !wasRunning {
		return s, sp.Tick
	}
	return s, nil
}

// WithServerState hands the tab the app shell's shared state and the link it
// was read over; nil st means no snapshot has arrived yet. The returned command
// starts the running spinner when the tab goes from idle to running.
func (m QueueModel) WithServerState(st *viewmodel.State, link tabapi.ServerLink) (tea.Model, tea.Cmd) {
	m.server = m.server.withLink(st, link)
	if m.server.down() || m.server.olderServer {
		// Whatever the queue showed is stale; it refills on reconnect.
		m.rows.Epics, m.server.summaries, m.server.finished = nil, nil, nil
		m.loaded = m.loaded || m.server.olderServer
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
	hadFinished := len(m.server.finished) > 0
	m.server = m.server.applyState(st)
	m.rows.Checked = make(map[string]bool, len(st.Queue))
	m.rows.CheckOrder = make(map[string]uint64, len(st.Queue))
	for i, addr := range st.Queue {
		m.rows.Checked[addr] = true
		m.rows.CheckOrder[addr] = uint64(i + 1)
	}
	m.applyEpics(tabapi.EpicsFromViewModel(*st, viewmodel.Scope{}), true)
	var cmd tea.Cmd
	m.server, cmd = m.server.syncRunState(m.rows.Epics, m.implementSpinner)
	if !hadFinished && len(m.server.finished) > 0 {
		// The first finished entry has no chain yet; a delivery only reaches the
		// tab that is on screen.
		var tick tea.Cmd
		m.server, tick = m.server.restartFinishedTick()
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

// markConflictResolution shows a claimed conflict-resolution child as resolving
// conflicts and its parent as waiting on it. The child runs inside the
// parent's merge, so the server holds no run (and no claim time) of its own for
// it; its timer stays blank rather than borrowing the parent's. The child's raw
// status is checked: its rendered one reads blocked while the parent is not done.
func markConflictResolution(epic gxtickets.Epic, live map[string]rowfmt.LiveTicketState) {
	for _, t := range epic.Tickets {
		if t.Type != string(schema.TypeConflictResolution) || t.Parent == nil ||
			t.Status != string(schema.StatusClaimed) {
			continue
		}
		parent, ok := live[*t.Parent]
		if !ok {
			continue
		}
		parent.Phase, parent.WaitingOn = rowfmt.LivePhaseResolvingConflicts, t.DisplayNumber()
		live[*t.Parent] = parent
		live[t.Identifier] = rowfmt.LiveTicketState{Running: true, Phase: rowfmt.LivePhaseResolvingConflicts}
	}
}
