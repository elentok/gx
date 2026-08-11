package tickets

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/keys"
)

func sPress() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 's', Text: "s"}
}

// selectTicketRow moves the sidebar selection onto the first non-epic row
// (row 0 is always the epic header per visibleRows' ordering).
func selectTicketRow(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if r, ok := m.selectedRow(); !ok || r.isEpic() {
		t.Fatalf("expected selection on a ticket row, got row=%+v ok=%v", r, ok)
	}
	return m
}

func TestModel_ChangeStatusKeyOpensMenuWithoutLiveLoop(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: needs-answer\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m = selectTicketRow(t, m)

	updated, _ = m.Update(sPress())
	m = updated.(Model)

	if !m.statusMenuOpen {
		t.Fatalf("expected status menu open after 's'")
	}
	got := menuValues(m.statusMenu)
	want := []string{"open", "claimed", "needs-repair", "draft", "done"}
	assertSameSet(t, got, want)
}

func TestModel_ChangeStatusKeyMenuExcludesGxOwnedStatusesWhileEpicIsLive(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: needs-answer\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m = selectTicketRow(t, m)

	previousRegistry := ralphLoopRegistry
	r := newLoopRegistry(1)
	r.tryStart("my-epic", 0, 1)
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("my-epic", nil)
		ralphLoopRegistry = previousRegistry
	})

	updated, _ = m.Update(sPress())
	m = updated.(Model)

	if !m.statusMenuOpen {
		t.Fatalf("expected status menu open after 's'")
	}
	got := menuValues(m.statusMenu)
	want := []string{"open", "needs-repair", "draft"}
	assertSameSet(t, got, want)
	for _, v := range got {
		if v == "claimed" || v == "done" {
			t.Fatalf("expected live-run menu to exclude gx-owned statuses, got %v", got)
		}
	}
}

func TestModel_ChangeStatusKeyUnparksTicketBackToOpen(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: needs-answer\n\nBody.\n")
	path := ticketPath(root, "my-epic", "01-first-ticket.md")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m = selectTicketRow(t, m)

	updated, _ = m.Update(sPress())
	m = updated.(Model)

	cursorTo(t, &m, "open")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.statusMenuOpen {
		t.Fatalf("expected menu closed after accepting a choice")
	}
	if cmd == nil {
		t.Fatalf("expected a command applying the status change")
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if cmd != nil {
		updated, _ = m.Update(cmd())
		m = updated.(Model)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "status: open") {
		t.Fatalf("expected ticket written with status: open on disk, got:\n%s", string(raw))
	}

	ticket := m.epics[0].Tickets[0]
	if schema.Status(ticket.Status) != schema.StatusOpen {
		t.Fatalf("expected reloaded model to reflect status=open, got %q", ticket.Status)
	}
}

func TestModel_ChangeStatusKeyDismissibleWithoutWriting(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: needs-answer\n\nBody.\n")
	path := ticketPath(root, "my-epic", "01-first-ticket.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m = selectTicketRow(t, m)

	updated, _ = m.Update(sPress())
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape, Text: "esc"})
	m = updated.(Model)
	if m.statusMenuOpen {
		t.Fatalf("expected menu closed after esc")
	}
	if cmd != nil {
		t.Fatalf("expected no command from dismissing the menu")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected no write from dismissing the menu, before:\n%s\nafter:\n%s", before, after)
	}
}

func menuValues(menu components.MenuState) []string {
	values := make([]string, len(menu.Items))
	for i, item := range menu.Items {
		values[i] = item.Value
	}
	return values
}

func assertSameSet(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	seen := map[string]bool{}
	for _, v := range got {
		seen[v] = true
	}
	for _, v := range want {
		if !seen[v] {
			t.Fatalf("got %v, want %v (missing %q)", got, want, v)
		}
	}
}

func cursorTo(t *testing.T, m *Model, value string) {
	t.Helper()
	for i, item := range m.statusMenu.Items {
		if item.Value == value {
			m.statusMenu.Cursor = i
			return
		}
	}
	t.Fatalf("value %q not found in menu items %+v", value, m.statusMenu.Items)
}
