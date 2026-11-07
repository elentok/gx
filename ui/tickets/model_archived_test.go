package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tree"
)

// TestModel_ExpandKeyOnArchivedHeaderStartsLoadAndDeliveryPopulates covers the
// tab's wiring of the Archived section's lazy load (the section's own
// behavior is tested in the sidebar package): the expand key returns the load
// command, and delivering its result populates the sidebar.
func TestModel_ExpandKeyOnArchivedHeaderStartsLoadAndDeliveryPopulates(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteArchivedTicket(t, root, "old-epic", "01-old-ticket.md", "Status: done\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.sidebar.Tree.SetSelectedIndex(archivedSectionHeaderIndex(t, m))
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected expand of the Archived section to return a load command")
	}

	m = deliverCmd(t, m, cmd)

	if m.sidebar.ArchivedLazy.State() != tree.LazyLoaded {
		t.Fatalf("ArchivedLazy.State() = %v, want LazyLoaded", m.sidebar.ArchivedLazy.State())
	}
	if len(m.sidebar.ArchivedEpics) != 1 || m.sidebar.ArchivedEpics[0].Name != "old-epic" {
		t.Fatalf("ArchivedEpics = %+v, want one epic named old-epic", m.sidebar.ArchivedEpics)
	}
}
