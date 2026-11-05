package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

func TestModel_TCTogglesHideDoneTickets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected done ticket visible before 'tc', got:\n%s", m.View().Content)
	}
	if got := len(visibleRows(m)); got != 3 {
		t.Fatalf("expected 3 visible rows (epic + 2 tickets) before 'tc', got %d", got)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)

	if strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected done ticket hidden after 'tc', got:\n%s", m.View().Content)
	}
	if !strings.Contains(m.View().Content, "Second ticket") {
		t.Fatalf("expected open ticket still visible after 'tc', got:\n%s", m.View().Content)
	}
	if got := len(visibleRows(m)); got != 2 {
		t.Fatalf("expected 2 visible rows (epic + open ticket) after 'tc', got %d", got)
	}
	// Epic header counts read epic.Tickets directly, so the filter must not
	// change them.
	if !strings.Contains(m.View().Content, "(1 done / 2)") {
		t.Fatalf("expected epic header counts unaffected by 'tc', got:\n%s", m.View().Content)
	}

	// Toggling 'tc' again restores the done ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)

	if !strings.Contains(m.View().Content, "First ticket") {
		t.Fatalf("expected done ticket visible again after second 'tc', got:\n%s", m.View().Content)
	}
	if got := len(visibleRows(m)); got != 3 {
		t.Fatalf("expected 3 visible rows again after second 'tc', got %d", got)
	}
}

func TestModel_TCOnFullyDoneEpicHidesAllTicketsButKeepsEpicRow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "done-epic", "01-only-ticket.md", "Status: done\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// The Closed section starts collapsed by default; expand it (and the
	// epic itself, which has no default of its own) so its ticket row would
	// normally be visible.
	setCollapsedSection(&m, sectionComplete, false)
	setCollapsedEpic(&m, indexOfEpic(t, m, "done-epic"), false)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)

	rows := visibleRows(m)
	if len(rows) != 1 || !rows[0].isEpic() {
		t.Fatalf("expected only the epic row visible for a fully-done epic under 'tc', got %+v", rows)
	}
	if !strings.Contains(m.View().Content, "done-epic") {
		t.Fatalf("expected epic row to still render, got:\n%s", m.View().Content)
	}
}

func TestModel_UnrelatedTAndCSequencesUnaffectedByHideDoneChord(t *testing.T) {
	// not parallel-safe: t.Setenv (EDITOR) is process-wide
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	// "e", "t" is the edit-tab chord, sharing the 't' key with the
	// hide-complete chord's second key: it must still fire as edit-tab, not
	// toggle hideDone.
	t.Setenv("EDITOR", "true")
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected 'et' to still launch the edit-tab command")
	}
	if m.sidebar.hideDone {
		t.Fatalf("expected 'et' to leave hideDone untouched")
	}

	// A bare "c" (no preceding "t") must not toggle hideDone either.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)
	if m.sidebar.hideDone {
		t.Fatalf("expected a bare 'c' to leave hideDone untouched")
	}
}
