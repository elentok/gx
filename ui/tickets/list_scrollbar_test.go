package tickets

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
)

func TestModel_SidebarScrollbarAppearsOnlyWhenListOverflows(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	shortContent := m.View().Content
	if strings.Contains(shortContent, "┃") {
		t.Fatalf("expected no sidebar scrollbar thumb when list fits, got:\n%s", shortContent)
	}

	for i := 2; i <= 60; i++ {
		writeTicket(t, root, "epic", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
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
	root := t.TempDir()
	for i := 1; i <= 60; i++ {
		writeTicket(t, root, "epic", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m = updated.(Model)

	before := m.scrollOffset
	selected := m.selected
	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = updated.(Model)

	if m.scrollOffset <= before {
		t.Fatalf("expected mouse wheel down to increase scroll offset from %d, got %d", before, m.scrollOffset)
	}
	if m.selected != selected {
		t.Fatalf("expected mouse wheel to leave selection at %d, got %d", selected, m.selected)
	}
}

// TestModel_CtrlDCtrlUPagesSidebar mirrors the Queue tab's ctrl+d/ctrl+u
// paging test — both tabs must page by list.DefaultScroll rows.
func TestModel_CtrlDCtrlUPagesSidebar(t *testing.T) {
	root := t.TempDir()
	for i := 1; i <= 40; i++ {
		writeTicket(t, root, "epic", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	m = updated.(Model)

	if m.selected != 0 {
		t.Fatalf("expected initial selection at 0, got %d", m.selected)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.selected != list.DefaultScroll {
		t.Fatalf("expected ctrl+d to move selection by %d, got %d", list.DefaultScroll, m.selected)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.selected != 0 {
		t.Fatalf("expected ctrl+u to move selection back to 0, got %d", m.selected)
	}

	// Clamps at the top: ctrl+u past the start stays at 0.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.selected != 0 {
		t.Fatalf("expected ctrl+u to clamp at 0, got %d", m.selected)
	}
}

func TestQueueModel_ScrollbarAppearsOnlyWhenListOverflows(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-only-ticket.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "epic", "01-only-ticket.md"): true}

	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))
	shortContent := m.View().Content
	if strings.Contains(shortContent, "┃") {
		t.Fatalf("expected no queue scrollbar thumb when list fits, got:\n%s", shortContent)
	}

	checked2 := map[string]bool{}
	for i := 1; i <= 60; i++ {
		name := fmt.Sprintf("%02d-ticket.md", i)
		writeTicket(t, root, "epic2", name, "Status: open\n\nBody.\n")
		checked2[ticketPath(root, "epic2", name)] = true
	}
	m2 := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked2))
	longContent := m2.View().Content
	if !strings.Contains(longContent, "┃") {
		t.Fatalf("expected a queue scrollbar thumb when the list overflows, got:\n%s", longContent)
	}
}
