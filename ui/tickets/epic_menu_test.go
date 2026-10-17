package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
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

func TestEpicMenu_CurrentStatusIsDim(t *testing.T) {
	items := epicMenuItems(epicMenuInput{epic: tickets.Epic{Status: "open"}})
	if got := itemByLabel(t, items, "Set open").Disabled; got != "already open" {
		t.Errorf("Set open disabled = %q, want already open", got)
	}
	if got := itemByLabel(t, items, "Set draft").Disabled; got != "" {
		t.Errorf("Set draft disabled = %q, want enabled", got)
	}
}
