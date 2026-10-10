package tickets

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/viewmodel"
)

func loadedServerQueue(t *testing.T, status string) (QueueModel, tea.Cmd) {
	t.Helper()
	claimedAt := time.Time{}
	if status == "claimed" {
		claimedAt = time.Now()
	}
	return loadedServerQueueWith(t, status, claimedAt, false)
}

// deliverQueue hands a fresh Queue tab the state the app shell would build
// from snap and the queue order, as WithServerState does on every delivery.
func deliverQueue(t *testing.T, snap server.Snapshot, queue []server.QueueItem) (QueueModel, tea.Cmd) {
	t.Helper()
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.New(nil)).WithServerLink(fakeServerAPI{}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return deliverQueueTo(next.(QueueModel), snap, queue)
}

func deliverQueueTo(m QueueModel, snap server.Snapshot, queue []server.QueueItem) (QueueModel, tea.Cmd) {
	st := viewmodel.State{}.ApplySnapshot(snap)
	st = st.SetQueue(queue)
	next, cmd := m.WithServerState(&st)
	return next.(QueueModel), cmd
}

func loadedServerQueueWith(t *testing.T, status string, claimedAt time.Time, herdrDown bool) (QueueModel, tea.Cmd) {
	t.Helper()
	snap := server.Snapshot{Seq: 1, HerdrUnavailable: herdrDown, Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: status, ClaimedAt: claimedAt},
		{Address: "gx:alpha/02", Title: "Second", Status: "open", BlockedBy: []string{"gx:alpha/01"}},
	}}
	return deliverQueue(t, snap, []server.QueueItem{{Address: "gx:alpha/01"}, {Address: "gx:alpha/02"}})
}

func TestQueueServerMode_ClaimedTicketShowsRunningState(t *testing.T) {
	m, cmd := loadedServerQueue(t, "claimed")

	if !m.runningEpics["gx:alpha"] {
		t.Fatalf("runningEpics = %v, want gx:alpha running", m.runningEpics)
	}
	if got := m.queueRunState(); got != queueRunRunning {
		t.Errorf("queueRunState = %v, want running", got)
	}
	live, ok := m.live["gx:alpha"]["01"]
	if !ok || !live.running || live.startedAt.IsZero() {
		t.Errorf("live[01] = %+v, want running with a start time", live)
	}
	if cmd == nil {
		t.Error("no spinner tick returned when the tab went idle -> running")
	}
}

func TestQueueServerMode_RunningHeaderCountsFromSnapshot(t *testing.T) {
	m, _ := deliverQueue(t, server.Snapshot{Seq: 1, Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "done"},
		{Address: "gx:alpha/02", Title: "Second", Status: "claimed", ClaimedAt: time.Now()},
		{Address: "gx:alpha/03", Title: "Third", Status: "open"},
	}}, []server.QueueItem{{Address: "gx:alpha/02"}, {Address: "gx:alpha/03"}})

	got := m.queueHeaderTitle()
	if !strings.HasPrefix(got, "Queue · 1 of 3 done · ") || !strings.Contains(got, "implementing...") {
		t.Errorf("queueHeaderTitle() = %q, want \"Queue · 1 of 3 done · <spinner> implementing...\"", got)
	}
}

func TestQueueServerMode_RunningHeaderDropsCountWithoutTickets(t *testing.T) {
	m, _ := loadedServerQueue(t, "open")
	m.runningEpics = map[string]bool{"gx:missing": true}

	got := m.queueHeaderTitle()
	if strings.Contains(got, "of 0 done") || !strings.HasPrefix(got, "Queue · ") || !strings.Contains(got, "implementing...") {
		t.Errorf("queueHeaderTitle() = %q, want a running title with no count", got)
	}
}

func TestQueueServerMode_NoClaimedTicketStaysIdleWithServerCopy(t *testing.T) {
	m, _ := loadedServerQueue(t, "open")

	if got := m.queueRunState(); got != queueRunIdle {
		t.Fatalf("queueRunState = %v, want idle", got)
	}
	body := strings.Join(m.queueHeaderBodyLines(), "\n")
	if strings.Contains(body, "press enter") {
		t.Errorf("idle banner %q tells the user to press enter in server mode", body)
	}
}

func TestQueueServerMode_EnterNeverStartsInProcessRun(t *testing.T) {
	m, _ := loadedServerQueue(t, "open")

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm := next.(QueueModel)

	if len(nm.runningEpics) != 0 || nm.confirm.IsOpen {
		t.Errorf("enter opened an in-process run: running=%v confirm=%v",
			nm.runningEpics, nm.confirm.IsOpen)
	}
	_ = cmd
}

func TestQueueServerMode_TimerCountsFromServerClaimTime(t *testing.T) {
	claimed := time.Now().Add(-10 * time.Minute)
	m, _ := loadedServerQueueWith(t, "claimed", claimed, false)

	if got := m.live["gx:alpha"]["01"].startedAt; !got.Equal(claimed) {
		t.Errorf("startedAt = %v, want the server's claim time %v", got, claimed)
	}
}

func TestQueueServerMode_HerdrDownBanner(t *testing.T) {
	m, _ := loadedServerQueueWith(t, "open", time.Time{}, true)

	body := strings.Join(m.queueHeaderBodyLines(), "\n")
	if !strings.Contains(body, "herdr unavailable") {
		t.Errorf("header body %q does not say herdr is unavailable", body)
	}
}

// A conflict-resolution child runs inside its parent's land, so the server has
// no run for it: it still shows as resolving, and the parent as waiting on it.
func TestQueueServerMode_ConflictChildShowsResolvingAndParentWaits(t *testing.T) {
	m, _ := deliverQueue(t, server.Snapshot{Seq: 1, Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "claimed", ClaimedAt: time.Now()},
		{Address: "gx:alpha/01a", Title: "Conflict resolution for 01", Status: "claimed",
			Type: "conflict-resolution", Parent: "gx:alpha/01"},
	}}, []server.QueueItem{{Address: "gx:alpha/01"}})

	child := m.live["gx:alpha"]["01a"]
	if !child.running || child.phase != livePhaseResolvingConflicts {
		t.Errorf("live[01a] = %+v, want running and resolving conflicts", child)
	}
	parent := m.live["gx:alpha"]["01"]
	if !parent.running || parent.waitingOn != "01a" {
		t.Errorf("live[01] = %+v, want running and waiting on 01a", parent)
	}
	_, suffix, _ := renderLiveTicketRow(m.icons(), m.implementSpinner, m.epics[0].Tickets[0], parent, "")
	if suffix != "(waiting on 01a...)" {
		t.Errorf("parent suffix = %q, want %q", suffix, "(waiting on 01a...)")
	}
}

func TestQueueServerMode_ClaimedWithoutServerRunIsNotImplementing(t *testing.T) {
	m, _ := loadedServerQueueWith(t, "claimed", time.Time{}, false)

	if len(m.runningEpics) != 0 {
		t.Errorf("runningEpics = %v, want none: the server has no run for the claimed ticket", m.runningEpics)
	}
	if got := m.queueRunState(); got != queueRunIdle {
		t.Errorf("queueRunState = %v, want idle", got)
	}
}

// A later delivery replaces the rows with no reload: a claim shows up running.
func TestQueueServerMode_NewDeliveryUpdatesRunningStateWithoutReload(t *testing.T) {
	m, _ := loadedServerQueue(t, "open")
	if len(m.runningEpics) != 0 {
		t.Fatalf("runningEpics = %v, want idle before the claim", m.runningEpics)
	}

	m, cmd := deliverQueueTo(m, server.Snapshot{Seq: 2, Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "claimed", ClaimedAt: time.Now()},
		{Address: "gx:alpha/02", Title: "Second", Status: "open", BlockedBy: []string{"gx:alpha/01"}},
	}}, []server.QueueItem{{Address: "gx:alpha/01"}, {Address: "gx:alpha/02"}})

	if !m.runningEpics["gx:alpha"] || cmd == nil {
		t.Errorf("runningEpics = %v, cmd = %v, want running with a spinner tick", m.runningEpics, cmd)
	}
}

func TestQueueServerMode_ActivationRestartsSpinner(t *testing.T) {
	m, _ := loadedServerQueue(t, "claimed")

	next, cmd := m.Update(m.OnPageActivated()())
	_ = next
	if cmd == nil {
		t.Fatal("activation returned no spinner tick for a running queue")
	}

	idle, _ := loadedServerQueue(t, "open")
	if _, cmd := idle.Update(idle.OnPageActivated()()); cmd != nil {
		t.Error("activation started a spinner for an idle queue")
	}
}

func TestQueueServerMode_PausedFollowsDeliveredMode(t *testing.T) {
	m, _ := loadedServerQueue(t, "open")
	if m.paused {
		t.Fatal("paused before any mode change")
	}

	st := viewmodel.State{Mode: server.ModePaused}
	next, _ := m.WithServerState(&st)

	if !next.(QueueModel).paused {
		t.Error("paused = false after a delivered paused mode")
	}
}

func TestQueueServerMode_ReloadAsksShellForSnapshot(t *testing.T) {
	m, _ := loadedServerQueue(t, "open")

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})

	if cmd == nil {
		t.Fatal("R returned no command")
	}
	if _, ok := cmd().(ResnapshotRequestedMsg); !ok {
		t.Errorf("R produced %T, want ResnapshotRequestedMsg", cmd())
	}
}

func TestQueueServerMode_InitDoesNotFetch(t *testing.T) {
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.New(nil)).WithServerLink(fakeServerAPI{}, nil)

	if cmd := m.Init(); cmd != nil {
		t.Errorf("Init returned a command in server mode")
	}
}
