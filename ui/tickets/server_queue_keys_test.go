package tickets

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

func newMaintenanceQueue(t *testing.T, api tickettest.FakeServerAPI) QueueModel {
	t.Helper()
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.New(nil)).
		WithServerLink(api, nil).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(QueueModel)
	m.loaded = true
	m.rows.epics = []tickets.Epic{{Name: "gx:alpha", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Title: "First", Path: "gx:alpha/01", Status: "open"}}}}
	m.rows.checked = map[string]bool{"gx:alpha/01": true}
	m.clampSelected()
	m = selectFirstQueueTicketRow(t, m)
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
		budget wire.BudgetStatus
		paused bool
		want   serverPauseAction
	}{
		{"running", wire.BudgetStatus{}, false, serverActionPause},
		{"paused", wire.BudgetStatus{}, true, serverActionResume},
		{"soft latched", wire.BudgetStatus{BudgetPaused: true}, false, serverActionOverride},
		{"hard latched beats paused", wire.BudgetStatus{HardLatched: true}, true, serverActionOverride},
	} {
		if got := serverPauseActionFor(tc.budget, tc.paused); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestQueueServerKeys_PauseResumeOverrideReachVerbs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		budget wire.BudgetStatus
		paused bool
		want   string
	}{
		{"pause", wire.BudgetStatus{}, false, "pause"},
		{"resume", wire.BudgetStatus{}, true, "resume"},
		{"override", wire.BudgetStatus{BudgetPaused: true}, false, "override"},
	} {
		var calls []string
		m := newMaintenanceQueue(t, tickettest.FakeServerAPI{Calls: &calls, BudgetRes: tc.budget})
		m.server.paused = tc.paused
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
		{"approved", schema.DemoteSection("## Proposed Remedy\n\nrun it", "## Proposed Remedy", time.Now()), nil},
	} {
		var calls []string
		m := newMaintenanceQueue(t, tickettest.FakeServerAPI{Calls: &calls})
		m.rows.epics[0].Tickets[0].Body = tc.body
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
	api := tickettest.FakeServerAPI{Calls: &calls}
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
	api := tickettest.FakeServerAPI{RemoveRes: wire.QueueResult{Refused: true, Reason: wire.ReasonTicketLive, Message: "gx:alpha/01 has a live iteration"}}
	m := newMaintenanceQueue(t, api)
	msg := m.srv.cmdRemove([]string{"gx:alpha/01"})()
	got := msg.(serverRemovedMsg)
	if got.removed != 0 || !strings.Contains(got.problem, "live iteration") {
		t.Fatalf("got %+v", got)
	}
}

func TestServerMode_FinishAndPauseKeyIsFinishAndPause(t *testing.T) {
	var calls []string
	m := newServerModel(t).WithServer(tickettest.FakeServerAPI{Calls: &calls}).WithTickets(tickettest.FakeServerAPI{Calls: &calls}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}})
	next, _ := m.handleServerFinishAndPauseKey()
	m = next.(Model)
	if !m.confirm.IsOpen {
		t.Fatal("confirm not open")
	}
	_, cmd, _ := m.confirm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("accept produced no command")
	}
	if _, ok := cmd().(serverDoneMsg); !ok || !slices.Equal(calls, []string{"pause"}) {
		t.Fatalf("calls = %v", calls)
	}
}
