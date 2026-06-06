package log

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestEscFromListInSplitCollapsesAndResizesListPanel is a regression test for
// the bug where esc from the list panel in split mode left the list panel at
// its narrow split-mode width instead of expanding it to fill the terminal.
func TestEscFromListInSplitCollapsesAndResizesListPanel(t *testing.T) {
	t.Parallel()
	m := newTestModel()

	// Set the terminal size so layout math is concrete.
	{
		next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
		m = next.(Model)
	}

	// Enter split mode by simulating the same transition openSelected uses:
	// drive the split container directly (no real git repo needed here).
	m.split, _ = m.split.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m.withSyncedListSize()
	m = m.withSyncedDetailSize()

	if !m.split.IsSplit() {
		t.Fatal("expected split mode after enter")
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after enter")
	}

	// The list panel should be at the narrow split-mode width.
	splitListW, _ := m.split.ListSize()
	if m.listPanel.width != splitListW {
		t.Fatalf("list width in split mode = %d, want %d", m.listPanel.width, splitListW)
	}
	if splitListW >= 200 {
		t.Fatalf("split-mode list width should be < 200, got %d", splitListW)
	}

	// First esc: focus moves from detail back to the list; split stays open.
	{
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		m = next.(Model)
	}
	if !m.split.IsSplit() {
		t.Fatal("expected still in split mode after first esc")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused after first esc")
	}

	// Second esc: collapses the split.
	{
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		m = next.(Model)
	}
	if !m.split.IsCollapsed() {
		t.Fatal("expected collapsed state after second esc from list")
	}

	// Regression: the list panel must expand to fill the full terminal width.
	if m.listPanel.width != 200 {
		t.Fatalf("list panel width = %d after collapse, want 200 (full terminal width)", m.listPanel.width)
	}
}
