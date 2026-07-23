package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func TestModel_EditChordOpensSelectedTicketFile(t *testing.T) {
	t.Setenv("EDITOR", "true")
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Selection starts on the epic row; move down to the ticket row.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected ee to launch an editor command for the selected ticket")
	}
}

func TestModel_EditChordSplitVariants(t *testing.T) {
	t.Setenv("EDITOR", "true")
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	chords := []string{"s", "v", "t"}
	for _, second := range chords {
		t.Run("e"+second, func(t *testing.T) {
			m := NewModel(root, ui.Settings{}, keys.New(nil))
			m = deliverLoad(t, m)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m = updated.(Model)
			updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
			m = updated.(Model)

			updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
			m = updated.(Model)
			updated, cmd := m.Update(tea.KeyPressMsg{Text: second})
			_ = updated.(Model)
			if cmd == nil {
				t.Fatalf("expected e%s chord to return a non-nil cmd", second)
			}
		})
	}
}

func TestModel_EditChordOnEpicWithMapOpensMapFile(t *testing.T) {
	t.Setenv("EDITOR", "true")
	root := t.TempDir()
	writeMap(t, root, "my-epic", "# My epic\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Selection starts on the epic row itself.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected ee on a map epic to launch an editor command for map.md")
	}
}

func TestModel_EditChordOnPlainEpicIsNoOpWithWarning(t *testing.T) {
	t.Setenv("EDITOR", "true")
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Selection starts on the epic row, which has no map.md (plain epic).
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	_ = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected ee on a plain epic to return a warning cmd, not nil")
	}
}
