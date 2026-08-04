package tickets

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
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
