package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
)

// deliverFlatLoad runs FlatModel's Init cmd(s) and feeds their results back
// through Update, mirroring deliverLoad (model_test.go) for the tree-shaped
// Model.
func deliverFlatLoad(t *testing.T, m FlatModel) FlatModel {
	t.Helper()
	cmd := m.Init()
	if cmd == nil {
		return m
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		updated, _ := m.Update(msg)
		return updated.(FlatModel)
	}
	for _, sub := range batch {
		subMsg := sub()
		if _, isTick := subMsg.(flatTickMsg); isTick {
			continue // don't chase the recurring refresh tick in tests
		}
		updated, _ := m.Update(subMsg)
		m = updated.(FlatModel)
	}
	return m
}

func TestFlatModel_EditChordOpensSelectedTicketFile(t *testing.T) {
	t.Setenv("EDITOR", "true")
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewFlatModel(root, "my-epic", ui.Settings{})
	m = deliverFlatLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(FlatModel)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(FlatModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(FlatModel)
	if cmd == nil {
		t.Fatalf("expected ee to launch an editor command for the selected ticket")
	}
}

func TestFlatModel_EditChordSplitVariants(t *testing.T) {
	t.Setenv("EDITOR", "true")
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	for _, second := range []string{"s", "v", "t"} {
		t.Run("e"+second, func(t *testing.T) {
			m := NewFlatModel(root, "my-epic", ui.Settings{})
			m = deliverFlatLoad(t, m)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m = updated.(FlatModel)

			updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
			m = updated.(FlatModel)
			updated, cmd := m.Update(tea.KeyPressMsg{Text: second})
			_ = updated.(FlatModel)
			if cmd == nil {
				t.Fatalf("expected e%s chord to return a non-nil cmd", second)
			}
		})
	}
}

func TestFlatModel_EditChordCancelLeavesSelectionNavigable(t *testing.T) {
	t.Setenv("EDITOR", "true")
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewFlatModel(root, "my-epic", ui.Settings{})
	m = deliverFlatLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(FlatModel)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(FlatModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(FlatModel)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(FlatModel)
	if m.selected != 1 {
		t.Fatalf("selected = %d, want 1 (cancel-chord shouldn't swallow later navigation)", m.selected)
	}
}
