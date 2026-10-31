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
	"github.com/elentok/gx/ui/nav"
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

func TestEpicMenu_SetStatusWritesTheEpicFileAndPingsTheServer(t *testing.T) {
	var calls []string
	for name, api := range map[string]ServerAPI{"server": fakeServerAPI{calls: &calls}, "no server": nil} {
		t.Run(name, func(t *testing.T) {
			calls = nil
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
	m, _ := epicMenuModel(t, fakeServerAPI{calls: &[]string{}})
	if !menuHasReason(openEpicMenuOn(t, m, "gx:alpha"), ticketsNotDone) {
		t.Errorf("open menu has no item dim for %q", ticketsNotDone)
	}
	m.serverAPI = nil
	if !menuHasReason(openEpicMenuOn(t, m, "gx:alpha"), serverNotRunning) {
		t.Errorf("open menu has no item dim for %q", serverNotRunning)
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
	m = openEpicMenuOn(t, updated.(Model), "gx:alpha")
	if view := m.ticketMenu.View(); !strings.Contains(view, "Merge into main") {
		t.Errorf("view lacks %q:\n%s", "Merge into main", view)
	}
	if !menuHasReason(m, "already draft") {
		t.Errorf("open menu has no item dim for %q", "already draft")
	}
}

// queueEpicModel is a server-mode tab on gx:alpha with tickets in every
// status Add epic to queue tells apart; gx:alpha/06 is already queued.
func queueEpicModel(t *testing.T, api fakeServerAPI) Model {
	t.Helper()
	st := applySnapshot(viewmodel.State{}.SetQueue([]server.QueueItem{{Address: "gx:alpha/06"}}), server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "Done", Status: "done", EpicStatus: "open"},
		{Address: "gx:alpha/02", Title: "Draft", Status: "draft"},
		{Address: "gx:alpha/03", Title: "Open", Status: "open"},
		{Address: "gx:alpha/04", Title: "Claimed", Status: "claimed"},
		pausedRepairInfo("gx:alpha/05", "Paused"),
		{Address: "gx:alpha/06", Title: "Queued", Status: "open"},
		{Address: "gx:alpha/07", Title: "Blocked", Status: "open", BlockedBy: []string{"gx:alpha/02"}},
	}})
	next, _ := newServerModel(t).WithServer(api).WithServerState(&st, ServerLinkUp)
	m := next.(Model)
	expandOpenEpics(&m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model)
}

func TestEpicMenu_AddEpicToQueueSendsTheEpicAddressOnce(t *testing.T) {
	var adds []server.QueueItem
	m := queueEpicModel(t, fakeServerAPI{adds: &adds})
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

func TestEpicMenu_AddEpicToQueueDimReasons(t *testing.T) {
	open := tickets.Epic{Name: "alpha", Status: "open"}
	items := epicMenuItems(epicMenuInput{epic: open})
	if got := itemByLabel(t, items, "Add epic to queue").Disabled; got != nothingToQueue {
		t.Errorf("no queueable tickets: disabled = %q, want %q", got, nothingToQueue)
	}
	items = epicMenuItems(epicMenuInput{epic: open, queueable: true, serverReason: serverNotRunning})
	if got := itemByLabel(t, items, "Add epic to queue").Disabled; got != serverNotRunning {
		t.Errorf("no server: disabled = %q, want %q", got, serverNotRunning)
	}
	items = epicMenuItems(epicMenuInput{epic: open, queueable: true})
	if got := itemByLabel(t, items, "Add epic to queue").Disabled; got != "" {
		t.Errorf("queueable: disabled = %q, want enabled", got)
	}
}

func TestEpicMenu_AddEpicToQueueRefusedEpics(t *testing.T) {
	for _, c := range []struct {
		name string
		epic tickets.Epic
		want string
	}{
		{"follow-ups", tickets.Epic{Name: "follow-ups", Status: "open"}, followUpsNotQueueable},
		{"follow-ups address", tickets.Epic{Name: "gx:follow-ups", Status: "open"}, followUpsNotQueueable},
		{"follow-ups-archived", tickets.Epic{Name: "follow-ups-archived", Status: "open"}, followUpsNotQueueable},
		{"draft", tickets.Epic{Name: "alpha", Status: "draft"}, "epic is draft, not open"},
		{"done", tickets.Epic{Name: "alpha", Status: "done"}, "epic is done, not open"},
		{"cancelled", tickets.Epic{Name: "alpha", Status: "cancelled"}, "epic is cancelled, not open"},
	} {
		items := epicMenuItems(epicMenuInput{epic: c.epic, queueable: true})
		if got := itemByLabel(t, items, "Add epic to queue").Disabled; got != c.want {
			t.Errorf("%s: disabled = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestEpicMenu_MergeBlockVerbsComeFromActions(t *testing.T) {
	done := tickets.Epic{Name: "gx:alpha", Status: "open", Tickets: []tickets.Ticket{{Status: "done"}}}
	labels := func(actions []string) []string {
		var out []string
		for _, it := range epicMenuItems(epicMenuInput{epic: done, actions: actions}) {
			if strings.HasPrefix(it.Label, "Merge") || strings.HasPrefix(it.Label, "Stash") {
				out = append(out, it.Label)
			}
		}
		return out
	}
	if got, want := labels([]string{"stash-merge", "merge"}), []string{"Stash, merge and restore", "Merge again"}; !slices.Equal(got, want) {
		t.Errorf("dirty-trunk labels = %v, want %v", got, want)
	}
	if got, want := labels([]string{"merge"}), []string{"Merge again"}; !slices.Equal(got, want) {
		t.Errorf("needs-rebase labels = %v, want %v", got, want)
	}
	if got, want := labels(nil), []string{"Merge"}; !slices.Equal(got, want) {
		t.Errorf("no actions labels = %v, want %v", got, want)
	}
}

func TestEpicMenu_StashMergeRunsOnTheEpic(t *testing.T) {
	var calls []string
	m, _ := epicMenuModel(t, fakeServerAPI{calls: &calls})
	m.vm.Epics = []server.EpicSummary{{Address: "gx:alpha", Actions: []string{"stash-merge", "merge"}}}
	_, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:alpha"), "Stash, merge and restore")
	if cmd == nil {
		t.Fatal("Stash, merge and restore issued nothing")
	}
	cmd()
	if !slices.Equal(calls, []string{"stash-merge gx:alpha"}) {
		t.Errorf("calls = %v, want stash-merge gx:alpha", calls)
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
