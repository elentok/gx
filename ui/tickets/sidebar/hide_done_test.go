package sidebar

import (
	"strings"
	"testing"

	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

func TestHideDoneFiltersDoneTicketsAndRestoresThem(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")
	s := loadedSidebar(t, root)

	if !strings.Contains(view(s), "First ticket") {
		t.Fatalf("expected done ticket visible before hiding, got:\n%s", view(s))
	}
	if got := len(visibleRows(s)); got != 3 {
		t.Fatalf("expected 3 visible rows (epic + 2 tickets) before hiding, got %d", got)
	}

	s.HideDone = true
	s.ClampSelected(testEnv())

	content := view(s)
	if strings.Contains(content, "First ticket") {
		t.Fatalf("expected done ticket hidden, got:\n%s", content)
	}
	if !strings.Contains(content, "Second ticket") {
		t.Fatalf("expected open ticket still visible, got:\n%s", content)
	}
	if got := len(visibleRows(s)); got != 2 {
		t.Fatalf("expected 2 visible rows (epic + open ticket) while hidden, got %d", got)
	}
	// Epic header counts read epic.Tickets directly, so the filter must not
	// change them.
	if !strings.Contains(content, "(1 done / 2)") {
		t.Fatalf("expected epic header counts unaffected by the filter, got:\n%s", content)
	}

	s.HideDone = false
	s.ClampSelected(testEnv())

	if !strings.Contains(view(s), "First ticket") {
		t.Fatalf("expected done ticket visible again, got:\n%s", view(s))
	}
	if got := len(visibleRows(s)); got != 3 {
		t.Fatalf("expected 3 visible rows again, got %d", got)
	}
}

func TestHideDoneOnFullyDoneEpicHidesAllTicketsButKeepsEpicRow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "done-epic", "01-only-ticket.md", "Status: done\n\nBody.\n")
	s := loadedSidebar(t, root)

	// The Closed section starts collapsed by default; expand it (and the
	// epic itself, which has no default of its own) so its ticket row would
	// normally be visible.
	setCollapsedSection(&s, SectionComplete, false)
	setCollapsedEpic(&s, indexOfEpic(t, s, "done-epic"), false)

	s.HideDone = true
	s.ClampSelected(testEnv())

	rows := visibleRows(s)
	if len(rows) != 1 || !rows[0].IsEpic() {
		t.Fatalf("expected only the epic row visible for a fully-done epic, got %+v", rows)
	}
	if !strings.Contains(view(s), "done-epic") {
		t.Fatalf("expected epic row to still render, got:\n%s", view(s))
	}
}
