package tickets

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

func TestModel_SidebarScrollbarAppearsOnlyWhenListOverflows(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	shortContent := m.View().Content
	if strings.Contains(shortContent, "┃") {
		t.Fatalf("expected no sidebar scrollbar thumb when list fits, got:\n%s", shortContent)
	}

	for i := 2; i <= 60; i++ {
		tickettest.WriteTicket(t, root, "epic", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}
	m2 := NewModel(root, ui.Settings{}, keys.New(nil))
	m2 = deliverLoad(t, m2)
	updated, _ = m2.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 = updated.(Model)

	longContent := m2.View().Content
	if !strings.Contains(longContent, "┃") {
		t.Fatalf("expected a sidebar scrollbar thumb when the list overflows, got:\n%s", longContent)
	}
}

func TestModel_MouseWheelScrollsSidebarWithoutMovingSelection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 1; i <= 60; i++ {
		tickettest.WriteTicket(t, root, "epic", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m = updated.(Model)

	before := m.sidebar.Tree.ScrollOffset()
	selected := m.sidebar.Tree.SelectedIndex()
	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = updated.(Model)

	if m.sidebar.Tree.ScrollOffset() <= before {
		t.Fatalf("expected mouse wheel down to increase scroll offset from %d, got %d", before, m.sidebar.Tree.ScrollOffset())
	}
	if m.sidebar.Tree.SelectedIndex() != selected {
		t.Fatalf("expected mouse wheel to leave selection at %d, got %d", selected, m.sidebar.Tree.SelectedIndex())
	}
}

// TestModel_CtrlDCtrlUPagesSidebar mirrors the Queue tab's ctrl+d/ctrl+u
// paging test — both tabs must page by list.DefaultScroll rows.
func TestModel_CtrlDCtrlUPagesSidebar(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 1; i <= 40; i++ {
		tickettest.WriteTicket(t, root, "epic", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	m = updated.(Model)

	// Entry 0 is the "Open epics" section header; initial selection lands on
	// the epic below it.
	if m.sidebar.Tree.SelectedIndex() != 1 {
		t.Fatalf("expected initial selection at 1, got %d", m.sidebar.Tree.SelectedIndex())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	if want := 1 + list.DefaultScroll; m.sidebar.Tree.SelectedIndex() != want {
		t.Fatalf("expected ctrl+d to move selection by %d, got %d", list.DefaultScroll, m.sidebar.Tree.SelectedIndex())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.sidebar.Tree.SelectedIndex() != 1 {
		t.Fatalf("expected ctrl+u to move selection back to 1, got %d", m.sidebar.Tree.SelectedIndex())
	}

	// Clamps at the top: ctrl+u past the start stops at entry 0, the section
	// header (itself cursor-reachable).
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.sidebar.Tree.SelectedIndex() != 0 {
		t.Fatalf("expected ctrl+u to clamp at 0, got %d", m.sidebar.Tree.SelectedIndex())
	}
}
