package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/preview"
)

// TestModel_NavigationCanSelectSectionHeaders covers the 03a contract:
// unlike the pre-migration sidebar, "Open"/"Closed" section headers are now
// real, cursor-reachable rows — up/down navigation can land on them, and
// selectedRow() reports ok=false there since a header is neither an epic
// nor a ticket. Moving off a header lands back on a real epic/ticket row.
func TestModel_NavigationCanSelectSectionHeaders(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "done-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "open-epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Selection starts on open-epic, the first epic row.
	r, ok := m.sidebar.selectedRow()
	if !ok || !r.isEpic() || m.sidebar.epics[r.epicIdx].Name != "open-epic" {
		t.Fatalf("expected initial selection on open-epic, got row=%+v ok=%v", r, ok)
	}

	// Moving up reaches the "Open epics" header, which reports ok=false.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = updated.(Model)
	if _, ok := m.sidebar.selectedRow(); ok {
		t.Fatalf("expected moving up from open-epic to land on the Open header (ok=false)")
	}

	// Moving down lands back on open-epic, a real selectable row.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	r, ok = m.sidebar.selectedRow()
	if !ok || !r.isEpic() || m.sidebar.epics[r.epicIdx].Name != "open-epic" {
		t.Fatalf("expected selection on open-epic after moving off the header, got row=%+v ok=%v", r, ok)
	}

	// Continuing down reaches open-epic's own ticket, then the Closed
	// header (done-epic itself stays hidden inside the collapsed Closed
	// section) — landing back on a header reports ok=false again.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if r, ok := m.sidebar.selectedRow(); !ok || r.isEpic() {
		t.Fatalf("expected selection on open-epic's ticket, got row=%+v ok=%v", r, ok)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if _, ok := m.sidebar.selectedRow(); ok {
		t.Fatalf("expected selection to land on the Closed header (ok=false)")
	}

	// Moving back up returns to the ticket row, not the header.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = updated.(Model)
	if r, ok := m.sidebar.selectedRow(); !ok || r.isEpic() {
		t.Fatalf("expected moving up from the Closed header to land back on open-epic's ticket, got row=%+v ok=%v", r, ok)
	}
}

// TestModel_MouseClickSelectsSidebarRowOnly covers ticket 05c: clicking a
// row in the Tickets tab's sidebar must select it (like arrowing there) and
// do nothing else — no checkbox toggle, no confirm modal.
func TestModel_MouseClickSelectsSidebarRowOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Navigate to the second ticket row: entry 0 is the "Open epics" header,
	// entry 1 its epic (where selection starts), entries 2/3 its two tickets
	// — two downs from the epic lands on the second ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.sidebar.tree.SelectedIndex() != 3 {
		t.Fatalf("expected selection at entry 3 after two downs, got %d", m.sidebar.tree.SelectedIndex())
	}
	line := selectedSidebarLine(t, m)

	// Entry 1 (the epic) is the reset baseline below.
	m.sidebar.tree.SetSelectedIndex(1)
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: line + 1})
	m = updated.(Model)
	if m.sidebar.tree.SelectedIndex() != 3 {
		t.Fatalf("expected click to select entry 3, got %d", m.sidebar.tree.SelectedIndex())
	}
	if m.confirm.IsOpen {
		t.Fatalf("expected click-to-select to not open the confirm modal")
	}

	// A click landing on the preview panel (past the sidebar's width) is a no-op.
	sidebarW, _ := m.splitWidth()
	m.sidebar.tree.SetSelectedIndex(1)
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: sidebarW, Y: line + 1})
	m = updated.(Model)
	if m.sidebar.tree.SelectedIndex() != 1 {
		t.Fatalf("expected click on the preview panel to leave selection at entry 1, got %d", m.sidebar.tree.SelectedIndex())
	}

}

// TestModel_RightClickOpensContextMenu covers ticket 12: a right click on a
// ticket or epic row selects it and opens the menu "m" opens; on the preview
// panel, a blank area or a group header it does nothing.
func TestModel_RightClickOpensContextMenu(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Entry 0 is the "Open epics" header, 1 the epic, 2/3 its tickets.
	lineOf := func(idx int) int {
		m.sidebar.tree.SetSelectedIndex(idx)
		return selectedSidebarLine(t, m)
	}
	headerLine, epicLine, ticketLine := lineOf(0), lineOf(1), lineOf(3)
	m.sidebar.tree.SetSelectedIndex(2)

	sidebarW, _ := m.splitWidth()
	for name, click := range map[string]tea.MouseClickMsg{
		"preview": {Button: tea.MouseRight, X: sidebarW, Y: ticketLine + 1},
		"blank":   {Button: tea.MouseRight, X: 0, Y: 38},
		"header":  {Button: tea.MouseRight, X: 0, Y: headerLine + 1},
	} {
		updated, cmd := m.Update(click)
		got := updated.(Model)
		if got.sidebar.tree.SelectedIndex() != 2 || got.menus.Menu.IsOpen || got.menus.Pending != "" || cmd != nil {
			t.Errorf("%s: expected a right click to do nothing, got selection=%d menuOpen=%v pending=%q cmd=%v",
				name, got.sidebar.tree.SelectedIndex(), got.menus.Menu.IsOpen, got.menus.Pending, cmd != nil)
		}
	}

	updated, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseRight, X: 0, Y: ticketLine + 1})
	m = updated.(Model)
	if m.sidebar.tree.SelectedIndex() != 3 {
		t.Fatalf("expected right click to select entry 3, got %d", m.sidebar.tree.SelectedIndex())
	}
	if want := tickettest.TicketPath(root, "my-epic", "02-second-ticket.md"); m.menus.Pending != want || cmd == nil {
		t.Fatalf("expected the ticket menu resolving for %q, got pending=%q cmd=%v", want, m.menus.Pending, cmd != nil)
	}

	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseRight, X: 0, Y: epicLine + 1})
	m = updated.(Model)
	if m.sidebar.tree.SelectedIndex() != 1 || !m.menus.Menu.IsOpen || !strings.Contains(m.menus.Menu.View(), "Add epic to queue") {
		t.Fatalf("expected the epic menu open on entry 1, got selection=%d view:\n%s", m.sidebar.tree.SelectedIndex(), m.menus.Menu.View())
	}
}

// TestModel_EnterOnExpandedEpicFocusesPreview covers the epic-row case of
// "enter"/"l": since my-epic starts expanded, enter must not collapse it —
// it should instead focus the preview panel, exactly like it does on a
// ticket row. Collapsing an epic is "h"/left's job only (see
// TestModel_HLCollapseAndExpandSelectedEpic).
func TestModel_EnterOnExpandedEpicFocusesPreview(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected ticket visible before enter, got:\n%s", m.View().Content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected epic to stay expanded after enter, got:\n%s", m.View().Content)
	}
	if m.preview.Focus != preview.FocusPreview {
		t.Fatalf("expected enter on already-expanded epic to focus preview, got focus=%v", m.preview.Focus)
	}
}

// TestModel_EnterOnCollapsedEpicExpandsThenFocusesPreview covers the other
// half: on a collapsed epic, the first enter expands it (staying on the
// sidebar so the user can see what appeared); only a second enter, now that
// it's expanded, focuses the preview panel.
func TestModel_EnterOnCollapsedEpicExpandsThenFocusesPreview(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected ticket hidden after 'h' collapse, got:\n%s", m.View().Content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected first enter to expand the collapsed epic, got:\n%s", m.View().Content)
	}
	if m.preview.Focus != preview.FocusSidebar {
		t.Fatalf("expected first enter on collapsed epic to keep sidebar focus, got focus=%v", m.preview.Focus)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.preview.Focus != preview.FocusPreview {
		t.Fatalf("expected second enter on now-expanded epic to focus preview, got focus=%v", m.preview.Focus)
	}
}

func TestModel_HLCollapseAndExpandSelectedEpic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected ticket hidden after 'h' collapse, got:\n%s", m.View().Content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected ticket visible after 'l' expand, got:\n%s", m.View().Content)
	}
}

func TestModel_NavigationSkipsCollapsedEpicTickets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "epic-a", "01-first-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "epic-b", "01-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Rows so far: [header, epic-a (selected), first-ticket, epic-b,
	// second-ticket]. Collapse epic-a, then moving down once should land on epic-b, not its
	// now-hidden ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	r, ok := m.sidebar.selectedRow()
	if !ok || !r.isEpic() || m.sidebar.epics[r.epicIdx].Name != "epic-b" {
		t.Fatalf("expected selection to land on epic-b after collapsing epic-a, got row=%+v ok=%v", r, ok)
	}
}

func TestModel_NoGlobalCollapseExpandAllBinding(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "epic-a", "01-first-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "epic-b", "01-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Selection starts on epic-a. Collapsing it must not affect epic-b.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "Second ticket") {
		t.Fatalf("expected epic-b's ticket unaffected by collapsing epic-a, got:\n%s", m.View().Content)
	}
}

func TestModel_RRefreshesDataFromDisk(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if strings.Contains(m.View().Content, "Second ticket") {
		t.Fatalf("expected second ticket absent before it's written, got:\n%s", m.View().Content)
	}

	// Simulate an edit made outside gx (e.g. in $EDITOR): a new ticket file
	// appears on disk after the tab already loaded.
	tickettest.WriteTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a refresh cmd from pressing R")
	}
	m = deliverCmd(t, m, cmd)

	if !strings.Contains(m.View().Content, "Second ticket") {
		t.Fatalf("expected second ticket visible after R refresh, got:\n%s", m.View().Content)
	}
}
