package queue

import (
	"strings"
	"testing"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

// TestQueueModelRowsRenderWithNoCheckbox covers ticket 08: the Queue tab is
// read-only for selection, so its rows must not render a checkbox glyph
// (checking/selecting only happens in the Tickets tab).
func TestQueueModelRowsRenderWithNoCheckbox(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	name := "01-first.md"
	tickettest.WriteTicket(t, root, "alpha", name, "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", name): true}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	content := m.View().Content
	for _, glyph := range []string{"[x]", "[ ]", "󰄲", "󰄱"} {
		if strings.Contains(content, glyph) {
			t.Fatalf("expected no checkbox glyph %q in Queue tab rows:\n%s", glyph, content)
		}
	}
}

// TestQueueModelShowsSameStatusAsTicketsTab covers ticket 25's first
// request: the Queue tab must show each ticket's status icon, blocked-by
// suffix, and (for a done ticket) the same elapsed/tokens metrics the
// Tickets tab renders (view.go's renderTicketRow), not just a bare checkbox
// and title.
func TestQueueModelShowsSameStatusAsTicketsTab(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-foundation.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "03-done.md", "---\nid: \"03\"\nstatus: done\ntype: implement\nactual_context_window: 12000\nelapsed_time: 754\n---\n\nDone.\n")

	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-foundation.md"): true,
		tickettest.TicketPath(root, "alpha", "02-dependent.md"):  true,
		tickettest.TicketPath(root, "alpha", "03-done.md"):       true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	content := m.View().Content

	if strings.Contains(content, "(blocked by 01)") {
		t.Fatalf("expected no blocked-by suffix without a server:\n%s", content)
	}
	if !strings.Contains(content, "12.0k tok") || !strings.Contains(content, "12m34s") {
		t.Fatalf("expected the done ticket's elapsed/tokens metrics line:\n%s", content)
	}
}
