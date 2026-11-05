package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tree"
)

func TestModel_ClickOnEpicIconTogglesIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	const epicIdx = 1
	epic := m.sidebar.Tree.Entries()[epicIdx]
	iconX := rowfmt.PanelPaddingX + tree.LabelColumn(epic)
	before := len(m.sidebar.Tree.Entries())

	// A click far right of the icon only selects.
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: iconX + 10, Y: epicIdx + 1})
	m = updated.(Model)
	if got := len(m.sidebar.Tree.Entries()); got != before {
		t.Fatalf("click off the icon changed entries: %d -> %d", before, got)
	}

	for _, x := range []int{iconX, iconX - 1, iconX + 1} {
		updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: epicIdx + 1})
		m = updated.(Model)
		if got := len(m.sidebar.Tree.Entries()); got != before-1 {
			t.Fatalf("x=%d: click on icon should collapse the epic, entries %d -> %d", x, before, got)
		}
		if m.sidebar.Tree.SelectedIndex() != epicIdx {
			t.Fatalf("x=%d: click should select the epic row, got %d", x, m.sidebar.Tree.SelectedIndex())
		}
		updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: epicIdx + 1})
		m = updated.(Model)
		if got := len(m.sidebar.Tree.Entries()); got != before {
			t.Fatalf("x=%d: second click should re-expand, entries %d, want %d", x, got, before)
		}
	}
}
