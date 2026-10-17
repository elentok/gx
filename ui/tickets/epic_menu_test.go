package tickets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/viewmodel"
)

// epicMenuModel is a server-mode tab whose gx:alpha epic has a ticket.md in
// a temp store, with gx:alpha/01 done and gx:alpha/02 open.
func epicMenuModel(t *testing.T, api fakeServerAPI) (Model, string) {
	t.Helper()
	store := t.TempDir()
	writeStoreTicket(t, store, "01-a.md", "---\nid: \"01\"\nstatus: done\ntype: research\n---\n\n# 01 — A\n\nBody.\n")
	epicMD := filepath.Join(store, "gx", "alpha", "ticket.md")
	if err := os.WriteFile(epicMD, []byte("---\nstatus: draft\n---\n\n# alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newServerModel(t).WithServer(api)
	m.settings.TicketStore = store
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "A", Status: "done"},
		{Address: "gx:alpha/02", Title: "B", Status: "open"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model), epicMD
}

func TestEpicMenu_SetStatusWritesTheEpicFile(t *testing.T) {
	for name, api := range map[string]ServerAPI{"server": fakeServerAPI{calls: &[]string{}}, "no server": nil} {
		t.Run(name, func(t *testing.T) {
			m, epicMD := epicMenuModel(t, fakeServerAPI{calls: &[]string{}})
			m.serverAPI = api
			_, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:alpha"), "Set open")
			if cmd == nil {
				t.Fatal("Set open issued nothing")
			}
			if msg := cmd(); msg != (statusChangedMsg{}) {
				t.Fatalf("Set open = %#v", msg)
			}
			if raw, _ := os.ReadFile(epicMD); !strings.Contains(string(raw), "status: open") {
				t.Errorf("ticket.md = %s, want status: open", raw)
			}
		})
	}
}

func TestEpicMenu_CheckAllChecksEveryTicket(t *testing.T) {
	m, _ := epicMenuModel(t, fakeServerAPI{calls: &[]string{}})
	m, _ = chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:alpha"), "Check all")
	if !m.isChecked("gx:alpha/02") {
		t.Errorf("checked = %v, want gx:alpha/02", m.checked)
	}
	if !m.epicChecked(m.epics[0]) {
		t.Error("epic not fully checked")
	}
}

func TestEpicMenu_MergeDimReasons(t *testing.T) {
	m, _ := epicMenuModel(t, fakeServerAPI{calls: &[]string{}})
	if view := openEpicMenuOn(t, m, "gx:alpha").ticketMenu.View(); !strings.Contains(view, ticketsNotDone) {
		t.Errorf("view lacks %q:\n%s", ticketsNotDone, view)
	}
	m.serverAPI = nil
	if view := openEpicMenuOn(t, m, "gx:alpha").ticketMenu.View(); !strings.Contains(view, serverNotRunning) {
		t.Errorf("view lacks %q:\n%s", serverNotRunning, view)
	}
}

func TestEpicMenu_MergeLabelNamesTheBranch(t *testing.T) {
	items := epicMenuItems(epicMenuInput{branch: "main"})
	itemByLabel(t, items, "Merge into main")
}

func TestEpicMenu_SnapshotNamesTheBranchAndDimsTheStatus(t *testing.T) {
	m := withState(newServerModel(t).WithServer(fakeServerAPI{calls: &[]string{}}), server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "A", Status: "open", EpicStatus: "draft", EpicMergeTarget: "main"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := openEpicMenuOn(t, updated.(Model), "gx:alpha").ticketMenu.View()
	for _, want := range []string{"Merge into main", "already draft"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
}

// queueEpicModel is a server-mode tab on gx:alpha with tickets in every
// status Add epic to queue tells apart; gx:alpha/06 is already queued.
func queueEpicModel(t *testing.T, api fakeServerAPI) Model {
	t.Helper()
	st := viewmodel.State{}.SetQueue([]server.QueueItem{{Address: "gx:alpha/06"}}).ApplySnapshot(server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "Done", Status: "done"},
		{Address: "gx:alpha/02", Title: "Draft", Status: "draft"},
		{Address: "gx:alpha/03", Title: "Open", Status: "open"},
		{Address: "gx:alpha/04", Title: "Claimed", Status: "claimed"},
		{Address: "gx:alpha/05", Title: "Parked", Status: "needs-repair"},
		{Address: "gx:alpha/06", Title: "Queued", Status: "open"},
		{Address: "gx:alpha/07", Title: "Blocked", Status: "open", BlockedBy: []string{"gx:alpha/02"}},
	}})
	next, _ := newServerModel(t).WithServer(api).WithServerState(&st, ServerLinkUp)
	m := next.(Model)
	expandOpenEpics(&m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model)
}

func TestEpicMenu_AddEpicToQueueAddsOpenUnqueuedTicketsInOrder(t *testing.T) {
	var adds []server.QueueItem
	m := queueEpicModel(t, fakeServerAPI{adds: &adds, addRes: map[string]server.QueueResult{
		"gx:alpha/07": {Refused: true, Message: "blocked by gx:alpha/02"},
	}})
	m, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:alpha"), "Add epic to queue")
	if cmd == nil {
		t.Fatal("Add epic to queue issued nothing")
	}
	_, toastCmd := m.Update(cmd())
	var got []string
	for _, a := range adds {
		got = append(got, a.Address)
	}
	if want := []string{"gx:alpha/03", "gx:alpha/07"}; !slices.Equal(got, want) {
		t.Errorf("queue-adds = %v, want %v", got, want)
	}
	toast, ok := toastCmd().(notify.NotifyMsg)
	if want := "queued 1, refused 1 (gx:alpha/07: blocked by gx:alpha/02)"; !ok || toast.Message != want {
		t.Errorf("toast = %#v, want %q", toast, want)
	}
}

func TestEpicMenu_AddEpicToQueueDimReasons(t *testing.T) {
	items := epicMenuItems(epicMenuInput{})
	if got := itemByLabel(t, items, "Add epic to queue").Disabled; got != nothingToQueue {
		t.Errorf("no queueable tickets: disabled = %q, want %q", got, nothingToQueue)
	}
	items = epicMenuItems(epicMenuInput{queueable: []string{"gx:alpha/03"}, serverReason: serverNotRunning})
	if got := itemByLabel(t, items, "Add epic to queue").Disabled; got != serverNotRunning {
		t.Errorf("no server: disabled = %q, want %q", got, serverNotRunning)
	}
	items = epicMenuItems(epicMenuInput{queueable: []string{"gx:alpha/03"}})
	if got := itemByLabel(t, items, "Add epic to queue").Disabled; got != "" {
		t.Errorf("queueable: disabled = %q, want enabled", got)
	}
}

func TestEpicMenu_CurrentStatusIsDim(t *testing.T) {
	items := epicMenuItems(epicMenuInput{epic: tickets.Epic{Status: "open"}})
	if got := itemByLabel(t, items, "Set open").Disabled; got != "already open" {
		t.Errorf("Set open disabled = %q, want already open", got)
	}
	if got := itemByLabel(t, items, "Set draft").Disabled; got != "" {
		t.Errorf("Set draft disabled = %q, want enabled", got)
	}
}
