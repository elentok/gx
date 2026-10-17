package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"
)

func TestOpenHelp_TicketsOpensHelpFromSearchInputWithoutTyping(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

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
	if m.search.Mode() != search.SearchModeNone || m.search.Query() != "" {
		t.Fatalf("search not ended: mode %v query %q", m.search.Mode(), m.search.Query())
	}
}

func TestOpenHelp_TicketsEndsPreviewSearch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.focus = focusPreview
	m.previewSearch.Start("body")
	updated, _ = m.Update(nav.OpenHelp()())
	m = updated.(Model)

	if !m.help.IsOpen {
		t.Fatal("OpenHelp did not open help")
	}
	if m.previewSearch.Mode() != search.SearchModeNone || m.previewSearch.Query() != "" {
		t.Fatalf("preview search not ended: mode %v query %q", m.previewSearch.Mode(), m.previewSearch.Query())
	}
}

func TestOpenHelp_TicketsIgnoredWhileModalOpen(t *testing.T) {
	t.Parallel()
	m := NewModel(t.TempDir(), ui.Settings{}, keys.New(nil))
	m.statusMenuOpen = true
	updated, _ := m.Update(nav.OpenHelp()())
	if updated.(Model).help.IsOpen {
		t.Fatal("OpenHelp opened help over the status menu")
	}
}

func TestOpenHelp_QueueOpensHelpFromSearchInputWithoutTyping(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	updated, _ = updated.Update(nav.OpenHelp()())
	m = updated.(QueueModel)

	if !m.help.IsOpen {
		t.Fatal("OpenHelp did not open help")
	}
	if m.search.Mode() != search.SearchModeNone || m.search.Query() != "" {
		t.Fatalf("search not ended: mode %v query %q", m.search.Mode(), m.search.Query())
	}
}

func TestOpenHelp_QueueIgnoredWhileActionsMenuOpen(t *testing.T) {
	t.Parallel()
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.Manager{})
	m.actionsMenu.IsOpen = true
	if !m.ModalOpen() {
		t.Fatal("the actions menu does not count as a modal")
	}
	updated, _ := m.Update(nav.OpenHelp()())
	if updated.(QueueModel).help.IsOpen {
		t.Fatal("OpenHelp opened help over the actions menu")
	}
}
