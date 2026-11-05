package tickets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/viewmodel"
)

// epicMenuModel is a server-mode tab whose gx:alpha epic has a ticket.md in
// a temp store, with gx:alpha/01 done and gx:alpha/02 open.
func epicMenuModel(t *testing.T, api tickettest.FakeServerAPI) (Model, string) {
	t.Helper()
	store := t.TempDir()
	tickettest.WriteStoreTicket(t, store, "01-a.md", "---\nid: \"01\"\nstatus: done\ntype: research\n---\n\n# 01 — A\n\nBody.\n")
	epicMD := filepath.Join(store, "gx", "alpha", "ticket.md")
	if err := os.WriteFile(epicMD, []byte("---\nstatus: draft\n---\n\n# alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newServerModel(t).WithServer(api).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api})
	m.settings.TicketStore = store
	m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "A", Status: "done"},
		{Address: "gx:alpha/02", Title: "B", Status: "open"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model), epicMD
}

func TestEpicMenu_SetStatusWritesTheEpicFileAndPingsTheServer(t *testing.T) {
	var calls []string
	for name, api := range map[string]tabapi.TicketsAPI{"server": tickettest.FakeServerAPI{Calls: &calls}, "no server": nil} {
		t.Run(name, func(t *testing.T) {
			calls = nil
			m, epicMD := epicMenuModel(t, tickettest.FakeServerAPI{Calls: &[]string{}})
			m.tickets = api
			_, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:alpha"), "Set open")
			if cmd == nil {
				t.Fatal("Set open issued nothing")
			}
			if msg := cmd(); msg != (rowactions.StatusChangedMsg{}) {
				t.Fatalf("Set open = %#v", msg)
			}
			if raw, _ := os.ReadFile(epicMD); !strings.Contains(string(raw), "status: open") {
				t.Errorf("ticket.md = %s, want status: open", raw)
			}
			var want []string
			if api != nil {
				want = []string{"changed gx:alpha"}
			}
			if !slices.Equal(calls, want) {
				t.Errorf("calls = %v, want %v", calls, want)
			}
		})
	}
}

func TestEpicMenu_MergeDimReasons(t *testing.T) {
	m, _ := epicMenuModel(t, tickettest.FakeServerAPI{Calls: &[]string{}})
	if !menuHasReason(openEpicMenuOn(t, m, "gx:alpha"), "tickets not done") {
		t.Errorf("open menu has no item dim for %q", "tickets not done")
	}
	m.server.api = nil
	if !menuHasReason(openEpicMenuOn(t, m, "gx:alpha"), rowactions.ServerNotRunning) {
		t.Errorf("open menu has no item dim for %q", rowactions.ServerNotRunning)
	}
}

func TestEpicMenu_SnapshotNamesTheBranchAndDimsTheStatus(t *testing.T) {
	m := withState(newServerModel(t).WithServer(tickettest.FakeServerAPI{Calls: &[]string{}}).WithTickets(tickettest.FakeServerAPI{Calls: &[]string{}}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{Calls: &[]string{}}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{Calls: &[]string{}}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{Calls: &[]string{}}}), wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "A", Status: "open", EpicStatus: "draft", EpicMergeTarget: "main"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = openEpicMenuOn(t, updated.(Model), "gx:alpha")
	if view := m.menus.Menu.View(); !strings.Contains(view, "Merge into main") {
		t.Errorf("view lacks %q:\n%s", "Merge into main", view)
	}
	if !menuHasReason(m, "already draft") {
		t.Errorf("open menu has no item dim for %q", "already draft")
	}
}

// queueEpicModel is a server-mode tab on gx:alpha with tickets in every
// status Add epic to queue tells apart; gx:alpha/06 is already queued.
func queueEpicModel(t *testing.T, api tickettest.FakeServerAPI) Model {
	t.Helper()
	st := applySnapshot(viewmodel.State{}.SetQueue([]wire.QueueItem{{Address: "gx:alpha/06"}}), wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "Done", Status: "done", EpicStatus: "open"},
		{Address: "gx:alpha/02", Title: "Draft", Status: "draft"},
		{Address: "gx:alpha/03", Title: "Open", Status: "open"},
		{Address: "gx:alpha/04", Title: "Claimed", Status: "claimed"},
		pausedRepairInfo("gx:alpha/05", "Paused"),
		{Address: "gx:alpha/06", Title: "Queued", Status: "open"},
		{Address: "gx:alpha/07", Title: "Blocked", Status: "open", BlockedBy: []string{"gx:alpha/02"}},
	}})
	next, _ := newServerModel(t).WithServer(api).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api}).WithServerState(&st, tabapi.ServerLinkUp)
	m := next.(Model)
	expandOpenEpics(&m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model)
}

func TestEpicMenu_AddEpicToQueueSendsTheEpicAddressOnce(t *testing.T) {
	var adds []wire.QueueItem
	m := queueEpicModel(t, tickettest.FakeServerAPI{Adds: &adds})
	m, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:alpha"), "Add epic to queue")
	if cmd == nil {
		t.Fatal("Add epic to queue issued nothing")
	}
	_, toastCmd := m.Update(cmd())
	var got []string
	for _, a := range adds {
		got = append(got, a.Address)
	}
	if want := []string{"gx:alpha"}; !slices.Equal(got, want) {
		t.Errorf("queue-adds = %v, want %v", got, want)
	}
	for _, a := range adds {
		if a.Agent != "" {
			t.Errorf("queue-add agent = %q, want empty (follows the default)", a.Agent)
		}
	}
	var toast notify.NotifyMsg
	var switched bool
	for _, c := range toastCmd().(tea.BatchMsg) {
		switch r := c().(type) {
		case notify.NotifyMsg:
			toast = r
		default:
			if vs, ok := nav.IsSwitch(r); ok && vs.Tab == nav.TabQueue {
				switched = true
			}
		}
	}
	if want := "queued 1, refused 0"; toast.Message != want {
		t.Errorf("toast = %#v, want %q", toast, want)
	}
	if !switched {
		t.Error("did not switch to the Queue tab")
	}
}

func TestEpicMenu_StashMergeRunsOnTheEpic(t *testing.T) {
	var calls []string
	m, _ := epicMenuModel(t, tickettest.FakeServerAPI{Calls: &calls})
	m.server.vm.Epics = []wire.EpicSummary{{Address: "gx:alpha", Actions: []string{"stash-merge", "merge"}}}
	_, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:alpha"), "Stash, merge and restore")
	if cmd == nil {
		t.Fatal("Stash, merge and restore issued nothing")
	}
	cmd()
	if !slices.Equal(calls, []string{"stash-merge gx:alpha"}) {
		t.Errorf("calls = %v, want stash-merge gx:alpha", calls)
	}
}
