package sidebar

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

func TestScrollWheelScrollsWithoutMovingSelection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 1; i <= 60; i++ {
		tickettest.WriteTicket(t, root, "epic", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}
	s := loadedSidebar(t, root)
	s.Tree.SetVisibleHeight(18)

	before := s.Tree.ScrollOffset()
	selected := s.Tree.SelectedIndex()
	s.ScrollWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})

	if s.Tree.ScrollOffset() <= before {
		t.Fatalf("expected mouse wheel down to increase scroll offset from %d, got %d", before, s.Tree.ScrollOffset())
	}
	if s.Tree.SelectedIndex() != selected {
		t.Fatalf("expected mouse wheel to leave selection at %d, got %d", selected, s.Tree.SelectedIndex())
	}
}
