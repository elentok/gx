package queue

import (
	"context"
	"fmt"
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

type (
	ServerStartedMsg   struct{ err error }
	ServerStartConfirm struct{}
)

// finishedRefreshInterval is how often the Recently finished times re-render.
const finishedRefreshInterval = time.Minute

// FinishedTickMsg re-renders the relative times of the Recently finished list.
type FinishedTickMsg struct{ Gen int }

func FinishedTick(gen int) tea.Cmd {
	return tea.Tick(finishedRefreshInterval, func(time.Time) tea.Msg { return FinishedTickMsg{Gen: gen} })
}

func CmdProbeRequested() tea.Msg { return tabapi.ProbeRequestedMsg{} }

// Server is what the Queue tab knows about the server: the delivered
// snapshot's derived state, the link it was read over and the clients that
// reach it. It is passed by value; methods return the next value.
type Server struct {
	// Live is epicName -> ticket identifier -> running state, derived from the
	// delivered state (SyncRunState), so the Queue tab's rows render the
	// running spinner+phase presentation (rowfmt.RenderLiveTicketRow).
	Live map[string]map[string]rowfmt.LiveTicketState
	// Summaries is the server's summary of each queued epic by epic name.
	Summaries    map[string]wire.EpicSummary
	RunningEpics map[string]bool
	// HarnessLabels is the snapshot's row label by ticket address.
	HarnessLabels map[string]string
	// WaitingLabels is the Queue agent rows' check-lock label by ticket address.
	WaitingLabels map[string]string
	// CodexRows marks the snapshot tickets whose label says codex, by address.
	CodexRows map[string]bool
	// HerdrDown says the server cannot launch agents; HerdrText names the
	// tickets waiting on herdr (empty falls back to the generic banner).
	HerdrDown bool
	HerdrText string
	// Finished is the snapshot's Recently finished list. FinishedGen numbers the
	// refresh tick chain: each page activation starts a new chain and the older
	// chain's pending tick is ignored.
	Finished    []wire.FinishedEpic
	FinishedGen int
	// Budget is the last snapshot's budget: the header's spend and the
	// root rows' cost.
	Budget wire.BudgetStatus
	Paused bool
	// ServerAPI/ServerStart/Link back the server-down banner. Link is only ever
	// set by WithServerState; a tab with no client is delivered ServerLinkDown,
	// so it shows the banner too.
	ServerAPI   tabapi.ServerAPI
	ServerStart func(context.Context) error
	Link        tabapi.ServerLink
	// OlderServer: see Model.olderServer.
	OlderServer bool
}

// Down reports the down banner state: the link the shell last delivered.
func (s Server) Down() bool { return s.Link == tabapi.ServerLinkDown }

// WithLink records the link and whether the server predates the snapshot.
func (s Server) WithLink(st *viewmodel.State, link tabapi.ServerLink) Server {
	s.Link = link
	s.OlderServer = st != nil && !s.Down() && st.OlderServer()
	return s
}

// ApplyState derives the server-owned fields from st.
func (s Server) ApplyState(st *viewmodel.State) Server {
	s.Summaries = make(map[string]wire.EpicSummary, len(st.Epics))
	for _, sum := range st.Epics {
		s.Summaries[sum.Address] = sum
	}
	s.HarnessLabels = make(map[string]string, len(st.Tickets))
	s.CodexRows = make(map[string]bool)
	for _, t := range st.Tickets {
		s.CodexRows[t.Address] = t.Harness == "codex"
		if l := ui.QueueHarnessLabel(t.Harness, t.Tier, t.Model, t.Effort, t.HarnessNote); l != "" {
			s.HarnessLabels[t.Address] = l
		}
	}
	s.WaitingLabels = make(map[string]string)
	for _, sess := range st.Sessions {
		if sess.Queue && sess.WaitingLabel != "" {
			s.WaitingLabels[sess.Ticket] = sess.WaitingLabel
		}
	}
	s.Finished = st.RecentlyFinished
	s.HerdrDown, s.HerdrText, s.Budget = st.HerdrUnavailable, st.HerdrText, st.Budget
	s.Paused = st.Mode == wire.QueueModePaused
	return s
}

// RestartFinishedTick starts a new refresh chain, orphaning the old one, or
// returns a nil command when there is no finished entry to refresh.
func (s Server) RestartFinishedTick() (Server, tea.Cmd) {
	s.FinishedGen++
	if len(s.Finished) == 0 {
		return s, nil
	}
	return s, FinishedTick(s.FinishedGen)
}

// UpdateDown handles the server-start messages; ok is false for any other msg.
func (s Server) UpdateDown(msg tea.Msg) (cmd tea.Cmd, ok bool) {
	switch msg := msg.(type) {
	case ServerStartConfirm:
		start := s.ServerStart
		return func() tea.Msg { return ServerStartedMsg{err: start(context.Background())} }, true
	case ServerStartedMsg:
		if msg.err != nil {
			return notify.Error("start server: " + msg.err.Error()), true
		}
		return CmdProbeRequested, true
	}
	return nil, false
}

// StartConfirm asks before starting the server; ok is false when the server is
// up or the tab has no way to start it.
func (s Server) StartConfirm() (confirm.Options, bool) {
	if !s.Down() || s.ServerStart == nil {
		return confirm.Options{}, false
	}
	return confirm.Options{
		Prompt:    "server is down — start it now?",
		AcceptCmd: func() tea.Msg { return ServerStartConfirm{} },
	}, true
}

// launchStepCount is the number of launch steps a run goes through.
const launchStepCount = 4

// SyncRunState derives the Queue tab's running state from the server's epic
// summaries: an epic runs when the server says so, and its active tickets are
// the ones with a run.
// It fills RunningEpics and Live for the header, row spinners and timers, and
// returns the spinner tick when the tab goes from idle to running. A ticket's
// timer counts from the server's claim time.
func (s Server) SyncRunState(epics []gxtickets.Epic, sp spinner.Model) (Server, tea.Cmd) {
	wasRunning := len(s.RunningEpics) > 0
	live := map[string]map[string]rowfmt.LiveTicketState{}
	running := map[string]bool{}
	for _, epic := range epics {
		sum := s.Summaries[epic.Name]
		if sum.State != wire.EpicStateRunning {
			continue
		}
		active := make(map[string]wire.SummaryTicket, len(sum.Active))
		for _, a := range sum.Active {
			active[a.Address] = a
		}
		for _, t := range epic.Tickets {
			a, ok := active[t.Path]
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
			state := rowfmt.LiveTicketState{Running: true, Phase: phase, StartedAt: a.ClaimedAt}
			if a.LaunchStep.Label != "" {
				state.Phase = rowfmt.LivePhaseLaunching
				state.LaunchStep = fmt.Sprintf("%s (%d/%d)", a.LaunchStep.Label, a.LaunchStep.Position, launchStepCount)
			}
			live[epic.Name][t.Identifier] = state
			running[epic.Name] = true
		}
		markConflictResolution(epic, live[epic.Name])
	}
	s.Live = live
	s.RunningEpics = running
	if len(running) > 0 && !wasRunning {
		return s, sp.Tick
	}
	return s, nil
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
