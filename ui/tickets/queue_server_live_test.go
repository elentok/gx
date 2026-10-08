package tickets

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func loadedServerQueue(t *testing.T, status string) (QueueModel, tea.Cmd) {
	t.Helper()
	claimedAt := time.Time{}
	if status == "claimed" {
		claimedAt = time.Now()
	}
	return loadedServerQueueWith(t, status, claimedAt, false)
}

func loadedServerQueueWith(t *testing.T, status string, claimedAt time.Time, herdrDown bool) (QueueModel, tea.Cmd) {
	t.Helper()
	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	api := fakeServerAPI{
		snap: server.Snapshot{Seq: 1, HerdrUnavailable: herdrDown, Tickets: []server.TicketInfo{
			{Address: "gx:alpha/01", Title: "First", Status: status, ClaimedAt: claimedAt},
			{Address: "gx:alpha/02", Title: "Second", Status: "open", BlockedBy: []string{"gx:alpha/01"}},
		}},
		queue: []server.QueueItem{{Address: "gx:alpha/01"}, {Address: "gx:alpha/02"}},
	}
	m := NewQueueModelWithStore(t.TempDir(), ui.Settings{}, keys.New(nil), store).WithServerLink(api, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	next, cmd := next.(QueueModel).Update(m.cmdLoadQueue()())
	return next.(QueueModel), cmd
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

	if len(nm.pendingEpics) != 0 || len(nm.runningEpics) != 0 || nm.confirm.IsOpen || nm.implementAgentMenuOpen {
		t.Errorf("enter opened an in-process run: pending=%v running=%v confirm=%v menu=%v",
			nm.pendingEpics, nm.runningEpics, nm.confirm.IsOpen, nm.implementAgentMenuOpen)
	}
	_ = cmd
}

func TestQueueServerMode_NoLocalReattachOrStrandedChecks(t *testing.T) {
	m, _ := loadedServerQueue(t, "open")

	if msg := m.cmdCheckStrandedPending(); msg != nil {
		t.Error("stranded-pending check ran in server mode")
	}
	if cmd := m.startAvailableEpics(); cmd != nil {
		t.Error("startAvailableEpics launched an in-process run in server mode")
	}
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

func TestQueueServerMode_ClaimedWithoutServerRunIsNotImplementing(t *testing.T) {
	m, _ := loadedServerQueueWith(t, "claimed", time.Time{}, false)

	if len(m.runningEpics) != 0 {
		t.Errorf("runningEpics = %v, want none: the server has no run for the claimed ticket", m.runningEpics)
	}
	if got := m.queueRunState(); got != queueRunIdle {
		t.Errorf("queueRunState = %v, want idle", got)
	}
}
