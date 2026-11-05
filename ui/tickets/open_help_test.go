package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

func TestOpenHelp_TicketsOpensHelpFromSearchInputWithoutTyping(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	updated, _ = updated.Update(nav.OpenHelp()())
	m = updated.(Model)

	if !m.help.IsOpen {
		t.Fatal("OpenHelp did not open help")
	}
	if s := m.sidebar.Search; s.Mode() != search.SearchModeNone || s.Query() != "" {
		t.Fatalf("search not ended: mode %v query %q", s.Mode(), s.Query())
	}
}

func TestOpenHelp_TicketsEndsPreviewSearch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.preview.Focus = preview.FocusPreview
	m.preview.Search.Start("body")
	updated, _ = m.Update(nav.OpenHelp()())
	m = updated.(Model)

	if !m.help.IsOpen {
		t.Fatal("OpenHelp did not open help")
	}
	if m.preview.Search.Mode() != search.SearchModeNone || m.preview.Search.Query() != "" {
		t.Fatalf("preview search not ended: mode %v query %q", m.preview.Search.Mode(), m.preview.Search.Query())
	}
}

func TestOpenHelp_TicketsIgnoredWhileModalOpen(t *testing.T) {
	t.Parallel()
	m := NewModel(t.TempDir(), ui.Settings{}, keys.New(nil))
	m.menus.Menu = m.menus.Menu.Open(rowactions.TicketTarget{}, "", nil)
	updated, _ := m.Update(nav.OpenHelp()())
	if updated.(Model).help.IsOpen {
		t.Fatal("OpenHelp opened help over the context menu")
	}
}
