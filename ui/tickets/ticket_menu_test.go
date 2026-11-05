package tickets

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// requestTicketMenu presses "m" and returns the ready message its cmd
// resolves, not yet delivered.
func requestTicketMenu(t *testing.T, m Model) (Model, rowactions.TicketReadyMsg) {
	t.Helper()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if cmd == nil {
		t.Fatal("m issued no cmd")
	}
	ready, ok := cmd().(rowactions.TicketReadyMsg)
	if !ok {
		t.Fatalf("m resolved to %T, want rowactions.TicketReadyMsg", ready)
	}
	return updated.(Model), ready
}

// chooseTicketMenuItem filters the open Context menu down to label, presses
// enter and feeds the pick back through Update. cmd is nil when the item is
// dim.
func chooseTicketMenuItem(t *testing.T, m Model, label string) (Model, tea.Cmd) {
	t.Helper()
	if !m.menus.Menu.IsOpen {
		t.Fatal("context menu is not open")
	}
	updated := tea.Model(m)
	for _, r := range label {
		updated, _ = updated.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		return updated.(Model), nil
	}
	updated, cmd = updated.Update(cmd())
	return updated.(Model), cmd
}

func itemByLabel(t *testing.T, items []contextmenu.Item[rowactions.TicketTarget], label string) contextmenu.Item[rowactions.TicketTarget] {
	t.Helper()
	i := slices.IndexFunc(items, func(it contextmenu.Item[rowactions.TicketTarget]) bool { return it.Label == label })
	if i < 0 {
		t.Fatalf("no %q item in %v", label, itemLabels(items))
	}
	return items[i]
}

// menuHasReason reports whether an item of the open menu is dim for reason.
func menuHasReason(m Model, reason string) bool {
	return slices.ContainsFunc(m.menus.Menu.Items(), func(it contextmenu.Item[rowactions.TicketTarget]) bool { return it.Disabled == reason })
}

func itemLabels(items []contextmenu.Item[rowactions.TicketTarget]) []string {
	labels := make([]string, len(items))
	for i, it := range items {
		labels[i] = it.Group.String() + "/" + it.Label
	}
	return labels
}

func orchestrationLabels() []string {
	return []string{"Unmute & Reopen", "Pause", "Cancel"}
}

func withMutes(info wire.TicketInfo, mutes []schema.MuteRecord) wire.TicketInfo {
	info.Mutes = mutes
	return info
}

func TestTicketMenu_GroupsAndPauseByAddress(t *testing.T) {
	var calls []string
	m := newServerModel(t).WithServer(tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionAnswer, wire.ActionResume)}).WithTickets(tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionAnswer, wire.ActionResume)}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionAnswer, wire.ActionResume)}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionAnswer, wire.ActionResume)}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionAnswer, wire.ActionResume)}})
	m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{
		pausedAnswerInfo("gx:alpha/01", "First"),
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = pressMenuKey(t, selectTicketRow(t, updated.(Model)))

	view := m.menus.Menu.View()
	for _, want := range []string{"Suggested", "Status", "Server", `Ticket "First"`, resumeLabel, "Set draft", "Pause", "Transcript"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu view lacks %q:\n%s", want, view)
		}
	}
	if !strings.Contains(m.View().Content, resumeLabel) {
		t.Error("tab view does not overlay the menu")
	}

	m, cmd := chooseTicketMenuItem(t, m, "Pause")
	if cmd == nil {
		t.Fatal("pause issued no cmd")
	}
	if msg, ok := cmd().(rowactions.ServerWriteMsg); !ok || msg.Problem != "" {
		t.Fatalf("pause result = %+v", msg)
	}
	if !slices.Equal(calls, []string{"pause gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
	if m.menus.Menu.IsOpen {
		t.Error("menu still open after a pick")
	}
}

func TestTicketMenu_UnmuteByAddress(t *testing.T) {
	var calls []string
	m := newServerModel(t).WithServer(tickettest.FakeServerAPI{Calls: &calls}).WithTickets(tickettest.FakeServerAPI{Calls: &calls}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}})
	m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{
		withMutes(pausedRepairInfo("gx:alpha/01", "First"), muted),
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))

	_, cmd := chooseTicketMenuItem(t, pressMenuKey(t, m), "Unmute")
	if cmd == nil {
		t.Fatal("unmute is dim for a muted server ticket")
	}
	cmd()
	if !slices.Equal(calls, []string{"unmute gx:alpha/01"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestTicketMenu_ServerTicketMutesDecideUnmute(t *testing.T) {
	for name, tt := range map[string]struct {
		mutes []schema.MuteRecord
		want  string
	}{
		"muted":   {muted, ""},
		"unmuted": {nil, "not muted"},
	} {
		t.Run(name, func(t *testing.T) {
			m := newServerModel(t).WithServer(tickettest.FakeServerAPI{}).WithTickets(tickettest.FakeServerAPI{}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{}})
			m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{
				withMutes(pausedRepairInfo("gx:alpha/01", "First"), tt.mutes),
			}})
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			_, ready := requestTicketMenu(t, selectTicketRow(t, updated.(Model)))
			item := itemByLabel(t, ready.Items, "Unmute & Reopen")
			if item.Disabled != tt.want {
				t.Errorf("unmute disabled = %q, want %q", item.Disabled, tt.want)
			}
			if tt.mutes != nil && item.Group != contextmenu.Suggested {
				t.Errorf("unmute group = %v, want Suggested", item.Group)
			}
		})
	}
}

func TestTicketMenu_SetStatusWritesTheFile(t *testing.T) {
	for _, tt := range []struct{ label, want string }{
		{"Set open", "status: open"},
		{"Set done", "status: done"},
	} {
		t.Run(tt.label, func(t *testing.T) {
			var calls []string
			m, file := serverModelWithStoreTicket(t, tickettest.FakeServerAPI{Calls: &calls}, true)
			_, cmd := chooseTicketMenuItem(t, pressMenuKey(t, m), tt.label)
			if cmd == nil {
				t.Fatalf("%s issued nothing", tt.label)
			}
			cmd()
			if raw, _ := os.ReadFile(file); !strings.Contains(string(raw), tt.want) {
				t.Errorf("ticket file = %s, want %s", raw, tt.want)
			}
		})
	}
}

func TestTicketMenu_NoServer_OrchestrationIsDim(t *testing.T) {
	for name, cut := range map[string]func(*Model){
		"no server api": func(m *Model) { m.serverAPI = nil },
		"link down":     func(m *Model) { m.link = tabapi.ServerLinkDown },
	} {
		t.Run(name, func(t *testing.T) {
			var calls []string
			m, file := serverModelWithStoreTicket(t, tickettest.FakeServerAPI{Calls: &calls}, false)
			r, _ := m.sidebar.SelectedRow()
			m.sidebar.Epics[r.EpicIdx].Tickets[r.TicketIdx].Mutes = muted
			cut(&m)

			m, ready := requestTicketMenu(t, m)
			for _, label := range orchestrationLabels() {
				if got := itemByLabel(t, ready.Items, label).Disabled; got != rowactions.ServerNotRunning {
					t.Errorf("%s disabled = %q, want %q", label, got, rowactions.ServerNotRunning)
				}
			}
			updated, _ := m.Update(ready)
			m = updated.(Model)
			if !menuHasReason(m, rowactions.ServerNotRunning) {
				t.Errorf("open menu has no item dim for %q", rowactions.ServerNotRunning)
			}

			before, _ := os.ReadFile(file)
			for _, label := range []string{"Pause", "Cancel", "Unmute"} {
				next, cmd := chooseTicketMenuItem(t, m, label)
				if cmd != nil {
					t.Errorf("%s issued a cmd", label)
				}
				if !next.menus.Menu.IsOpen {
					t.Errorf("%s closed the menu", label)
				}
			}
			if after, _ := os.ReadFile(file); string(after) != string(before) {
				t.Errorf("ticket file changed:\n%s", after)
			}
			if len(calls) != 0 {
				t.Errorf("calls = %v, want none", calls)
			}

			_, cmd := chooseTicketMenuItem(t, m, "Set open")
			if cmd == nil {
				t.Fatal("set open issued nothing")
			}
			cmd()
			if raw, _ := os.ReadFile(file); !strings.Contains(string(raw), "status: open") {
				t.Errorf("ticket file = %s, want status: open", raw)
			}
		})
	}
}

func TestTicketMenu_ReadOnlyLinkDimsWithItsReason(t *testing.T) {
	m, _ := serverModelWithStoreTicket(t, tickettest.FakeServerAPI{}, false)
	m.link = tabapi.ServerLinkReadOnly
	_, ready := requestTicketMenu(t, m)
	if got, want := itemByLabel(t, ready.Items, "Pause").Disabled, tabapi.ServerLinkReadOnly.DisabledReason(); got != want {
		t.Errorf("pause disabled =%q, want %q", got, want)
	}
}

// stubTicketEditor records the path and read-only flag each edit launches.
func stubTicketEditor(t *testing.T) *[]string {
	t.Helper()
	var launched []string
	orig := rowactions.LaunchTicketEditor
	rowactions.LaunchTicketEditor = func(_ string, _ ui.Settings, path string, _ terminalrun.SplitType, readOnly bool) tea.Cmd {
		launched = append(launched, fmt.Sprintf("%s ro=%v", path, readOnly))
		return func() tea.Msg { return nil }
	}
	t.Cleanup(func() { rowactions.LaunchTicketEditor = orig })
	return &launched
}

func TestTicketMenu_AgentAndEditCopyGroups(t *testing.T) {
	m, file := serverModelWithStoreTicket(t, tickettest.FakeServerAPI{}, false)
	m = pressMenuKey(t, m)
	view := m.menus.Menu.View()
	for _, want := range []string{"Agent", "Edit/copy", "Watch", "Transcript", "Investigate",
		"Edit file (vsplit)", "e v", "Yank epic - ticket", "y y", "Yank file path", "y f"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu view lacks %q:\n%s", want, view)
		}
	}

	launched := stubTicketEditor(t)
	_, cmd := chooseTicketMenuItem(t, m, "Edit file (tab)")
	if cmd == nil {
		t.Fatal("edit issued no cmd")
	}
	if want := []string{file + " ro=false"}; !slices.Equal(*launched, want) {
		t.Errorf("launched = %v, want %v", *launched, want)
	}

	for _, tt := range []struct{ label, chord string }{
		{"Yank epic - ticket", "yy"},
		{"Yank file path", "yf"},
	} {
		got := stubClipboard(t)
		closed, _ := chooseTicketMenuItem(t, m, tt.label)
		fromMenu := *got
		*got = ""
		next := tea.Model(closed)
		for _, r := range tt.chord {
			next, _ = next.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		if fromMenu == "" || fromMenu != *got {
			t.Errorf("%s copied %q, %s copies %q", tt.label, fromMenu, tt.chord, *got)
		}
	}
}

func TestTicketMenu_ArchivedRowOnlyReads(t *testing.T) {
	root := t.TempDir()
	tickettest.WriteArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: paused\nPause reason: spinning\n\nBody.\n")
	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectArchivedTicketRow(t, updated.(Model))

	m, ready := requestTicketMenu(t, m)
	var enabled []string
	for _, it := range ready.Items {
		switch {
		case it.Disabled == "":
			enabled = append(enabled, it.Label)
		case it.Label != "Watch" && it.Disabled != "archived":
			t.Errorf("%s disabled = %q, want %q", it.Label, it.Disabled, "archived")
		}
	}
	want := []string{"Transcript", "Edit file", "Edit file (split)", "Edit file (vsplit)", "Edit file (tab)", "Yank epic - ticket", "Yank file path"}
	if !slices.Equal(enabled, want) {
		t.Errorf("enabled = %v, want %v", enabled, want)
	}

	launched := stubTicketEditor(t)
	updated, _ = m.Update(ready)
	chooseTicketMenuItem(t, updated.(Model), "Edit file")
	if len(*launched) != 1 || !strings.HasSuffix((*launched)[0], "01-old-ticket.md ro=true") {
		t.Errorf("launched = %v, want the archived file read-only", *launched)
	}
}

func hasItem(items []contextmenu.Item[rowactions.TicketTarget], label string) bool {
	return slices.ContainsFunc(items, func(it contextmenu.Item[rowactions.TicketTarget]) bool { return it.Label == label })
}

func TestTicketsTab_StatusKeyOpensNothing(t *testing.T) {
	m, _ := serverModelWithStoreTicket(t, tickettest.FakeServerAPI{}, false)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if updated.(Model).ModalOpen() || cmd != nil {
		t.Errorf("s opened a modal (%v) or issued a cmd (%v)", updated.(Model).ModalOpen(), cmd != nil)
	}
}
