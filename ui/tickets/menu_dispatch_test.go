package tickets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/sidebar"
)

var muted = tickettest.Muted

// writeMenuTicketFile writes a paused (answer) ticket file and returns its path.
func writeMenuTicketFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-first.md")
	body := "---\nid: \"01\"\ntitle: First\nstatus: paused\npause_reason: blocked-pane\n---\n\n# A\n\n" + tickets.LegacyAnswerHeading + "\n\nWhich approach?\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// selectTicketRow selects the first visible ticket row.
func selectTicketRow(t *testing.T, m Model) Model {
	t.Helper()
	for i, e := range m.sidebar.Tree.Entries() {
		if e.Value.Kind == sidebar.NodeTicket {
			m.sidebar.Tree.SetSelectedIndex(i)
			break
		}
	}
	if r, ok := m.sidebar.SelectedRow(); !ok || r.IsEpic() {
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
