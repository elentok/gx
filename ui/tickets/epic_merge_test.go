package tickets

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

func menuHasMerge(m actionsMenuModel) bool {
	return m.IsOpen && slices.ContainsFunc(m.state.Items, func(it components.MenuItem) bool { return it.Value == actionMerge })
}

// chooseMerge moves the open menu's cursor onto "merge" and accepts it,
// returning the toast the merge cmd resolves to.
func chooseMerge(t *testing.T, update func(tea.Msg) tea.Cmd, menu actionsMenuModel) notify.NotifyMsg {
	t.Helper()
	for i, it := range menu.state.Items {
		if it.Value == actionMerge {
			for range i {
				update(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			break
		}
	}
	cmd := update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("choosing merge issued no cmd")
	}
	msg, ok := cmd().(notify.NotifyMsg)
	if !ok {
		t.Fatalf("merge cmd = %T, want a toast", msg)
	}
	return msg
}

func TestEpicMerge_TicketsTabOffersMergeOnCompleteUnmergedEpicOnly(t *testing.T) {
	var calls []string
	api := fakeServerAPI{calls: &calls, mergeRes: server.QueueResult{MergeTicket: "gx:alpha/02"}}
	m := withState(newServerModel(t).WithServer(api), server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "Done", Status: "done"},
		{Address: "gx:beta/01", Title: "Open", Status: "open"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	setCollapsedSection(&m, sectionComplete, false)

	selectEpicRow(t, &m, "gx:beta")
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if updated.(Model).actionsMenu.IsOpen {
		t.Error("menu opened on an open epic")
	}

	ticketRow, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if r, ok := ticketRow.(Model).selectedRow(); !ok || r.isEpic() {
		t.Fatalf("selected row = %+v, want gx:beta/01", r)
	}
	if got := pressMenuKey(t, ticketRow.(Model)); menuHasMerge(got.actionsMenu) {
		t.Error("merge offered on a ticket row")
	}

	selectEpicRow(t, &m, "gx:alpha")
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = updated.(Model)
	if !menuHasMerge(m.actionsMenu) {
		t.Fatalf("menu = %+v, want a merge item", m.actionsMenu)
	}
	toast := chooseMerge(t, func(msg tea.Msg) tea.Cmd {
		next, cmd := m.Update(msg)
		m = next.(Model)
		return cmd
	}, m.actionsMenu)
	if !slices.Equal(calls, []string{"merge gx:alpha"}) {
		t.Errorf("calls = %v, want [merge gx:alpha]", calls)
	}
	if !strings.Contains(toast.Message, "gx:alpha/02") {
		t.Errorf("toast = %q, want the forked merge ticket", toast.Message)
	}
}

func TestEpicMerge_ServerModeMergedEpicSitsUnderMergedWithNoMergeItem(t *testing.T) {
	api := fakeServerAPI{calls: &[]string{}}
	m := withState(newServerModel(t).WithServer(api), server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "Done", Status: "done", EpicMergedAt: time.Now()},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	if got := epicSection(m.epics[0]); got != sectionMerged {
		t.Fatalf("section = %v, want sectionMerged", got)
	}
	setCollapsedSection(&m, sectionMerged, false)

	selectEpicRow(t, &m, "gx:alpha")
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if menuHasMerge(updated.(Model).actionsMenu) {
		t.Error("merge offered on a merged epic")
	}
}
