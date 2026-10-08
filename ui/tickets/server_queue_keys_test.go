package tickets

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func newMaintenanceQueue(t *testing.T, api fakeServerAPI) QueueModel {
	t.Helper()
	m := NewQueueModelWithStore(t.TempDir(), ui.Settings{}, keys.New(nil), loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))).
		WithServerLink(api, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(QueueModel)
	m.loaded = true
	m.epics = []tickets.Epic{{Name: "gx:alpha", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Title: "First", Path: "gx:alpha/01", Status: "open"}}}}
	if err := m.queueStore.SetChecked([]string{"gx:alpha/01"}, true); err != nil {
		t.Fatal(err)
	}
	m.checked = map[string]bool{"gx:alpha/01": true}
	m.clampSelected()
	return m
}

// press sends a key and runs the resulting command chain one step at a time.
func pressQueue(t *testing.T, m QueueModel, key rune) QueueModel {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
	m = next.(QueueModel)
	for i := 0; cmd != nil && i < 3; i++ {
		msg := cmd()
		if _, isBatch := msg.(tea.BatchMsg); isBatch {
			break
		}
		next, cmd = m.Update(msg)
		m = next.(QueueModel)
	}
	return m
}

func accept(t *testing.T, m QueueModel) QueueModel {
	t.Helper()
	if !m.confirm.IsOpen {
		t.Fatal("confirm not open")
	}
	_, cmd, _ := m.confirm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("accept produced no command")
	}
	next, _ := m.Update(cmd())
	return next.(QueueModel)
}

// Seam D: "p" offers override when a budget latch holds, resume when paused,
// pause otherwise.
func TestServerPauseActionFor(t *testing.T) {
	for _, tc := range []struct {
		name   string
		budget server.BudgetStatus
		paused bool
		want   serverPauseAction
	}{
		{"running", server.BudgetStatus{}, false, serverActionPause},
		{"paused", server.BudgetStatus{}, true, serverActionResume},
		{"soft latched", server.BudgetStatus{BudgetPaused: true}, false, serverActionOverride},
		{"hard latched beats paused", server.BudgetStatus{HardLatched: true}, true, serverActionOverride},
	} {
		if got := serverPauseActionFor(tc.budget, tc.paused); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestQueueServerKeys_PauseResumeOverrideReachVerbs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		budget server.BudgetStatus
		paused bool
		want   string
	}{
		{"pause", server.BudgetStatus{}, false, "pause"},
		{"resume", server.BudgetStatus{}, true, "resume"},
		{"override", server.BudgetStatus{BudgetPaused: true}, false, "override"},
	} {
		var calls []string
		m := newMaintenanceQueue(t, fakeServerAPI{calls: &calls, budget: tc.budget})
		m.paused = tc.paused
		m = accept(t, pressQueue(t, m, 'p'))
		if !slices.Equal(calls, []string{tc.want}) {
			t.Errorf("%s: calls = %v", tc.name, calls)
		}
	}
}

// Seam D: "A" approves a row with a pending proposal and does nothing on one
// without.
func TestQueueServerKeys_ApproveNeedsPendingProposal(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want []string
	}{
		{"pending", "## Proposed Remedy\n\nrun it", []string{"approve gx:alpha/01"}},
		{"none", "## Notes\n\nnothing", nil},
	} {
		var calls []string
		m := newMaintenanceQueue(t, fakeServerAPI{calls: &calls})
		m.epics[0].Tickets[0].Body = tc.body
		m.clampSelected()
		m = pressQueue(t, m, 'A')
		if tc.want != nil {
			m = accept(t, m)
		} else if m.confirm.IsOpen {
			t.Errorf("%s: confirm opened", tc.name)
		}
		if !slices.Equal(calls, tc.want) {
			t.Errorf("%s: calls = %v, want %v", tc.name, calls, tc.want)
		}
	}
}

func TestQueueServerKeys_DequeueAndDelete(t *testing.T) {
	var calls []string
	api := fakeServerAPI{calls: &calls}
	m := pressQueue(t, newMaintenanceQueue(t, api), 'C')
	accept(t, m)
	m = pressQueue(t, newMaintenanceQueue(t, api), 'x')
	accept(t, m)
	want := []string{"remove gx:alpha/01", "remove gx:alpha/01"}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestQueueServerKeys_LiveRefusalIsAnErrorToast(t *testing.T) {
	api := fakeServerAPI{removeRes: server.QueueResult{Refused: true, Reason: server.ReasonTicketLive, Message: "gx:alpha/01 has a live iteration"}}
	m := newMaintenanceQueue(t, api)
	msg := m.cmdServerRemove([]string{"gx:alpha/01"})()
	got := msg.(serverRemovedMsg)
	if got.removed != 0 || !strings.Contains(got.problem, "live iteration") {
		t.Fatalf("got %+v", got)
	}
}

func TestServerMode_DrainKeyReachesDrainVerb(t *testing.T) {
	var calls []string
	m := newServerModel(t).WithServer(fakeServerAPI{calls: &calls})
	next, _ := m.handleDrainReplaceKey()
	m = next.(Model)
	if !m.confirm.IsOpen {
		t.Fatal("confirm not open")
	}
	_, cmd, _ := m.confirm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("accept produced no command")
	}
	if _, ok := cmd().(serverDoneMsg); !ok || !slices.Equal(calls, []string{"drain"}) {
		t.Fatalf("calls = %v", calls)
	}
}
