package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

// TestModel_TCTogglesHideDone covers the "tc" chord's wiring; what hiding
// does to the rows is tested in the sidebar package.
func TestModel_TCTogglesHideDone(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	for i, wantRows := range []int{2, 3} {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
		m = updated.(Model)
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
		m = updated.(Model)

		if got := len(visibleRows(m)); got != wantRows || m.sidebar.HideDone != (i == 0) {
			t.Fatalf("after 'tc' #%d: rows=%d (want %d), hideDone=%v", i+1, got, wantRows, m.sidebar.HideDone)
		}
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
	if m.sidebar.HideDone {
		t.Fatalf("expected 'et' to leave hideDone untouched")
	}

	// A bare "c" (no preceding "t") must not toggle hideDone either.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)
	if m.sidebar.HideDone {
		t.Fatalf("expected a bare 'c' to leave hideDone untouched")
	}
}
