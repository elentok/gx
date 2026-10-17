package tickets

import (
	"testing"

	"github.com/elentok/gx/ui/components"
)

// selectTicketRow moves the sidebar selection onto the first visible ticket
// row.
func selectTicketRow(t *testing.T, m Model) Model {
	t.Helper()
	for i, e := range m.sidebarTree.Entries() {
		if e.Value.kind == nodeTicket {
			m.sidebarTree.SetSelectedIndex(i)
			break
		}
	}
	if r, ok := m.selectedRow(); !ok || r.isEpic() {
		t.Fatalf("expected selection on a ticket row, got row=%+v ok=%v", r, ok)
	}
	return m
}

func menuValues(menu components.MenuState) []string {
	values := make([]string, len(menu.Items))
	for i, item := range menu.Items {
		values[i] = item.Value
	}
	return values
}
