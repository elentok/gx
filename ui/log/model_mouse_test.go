package log

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestClickOnDetailPaneFocusesItAndRoutesWheelToCommitDetail is a regression
// test for the split's detail pane not being click-focusable: clicking inside
// its bounds should hand it focus via splitview.SetDetailFocused, and a
// subsequent wheel event should then route through commit.Model's own
// handleMouseWheel instead of scrolling the log list.
func TestClickOnDetailPaneFocusesItAndRoutesWheelToCommitDetail(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows(commitRows(30))

	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = next.(Model)

	// Enter split mode (mirrors model_split_test.go's approach).
	m.split, _ = m.split.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m.withSyncedListSize()
	m = m.withSyncedDetailSize()
	// esc once to hand focus back to the list, so the click is what moves it.
	m.split = m.split.SetDetailFocused(false)

	if !m.split.IsListFocused() {
		t.Fatal("expected list focused before the click")
	}

	col, row, visible := m.split.DetailOrigin()
	if !visible {
		t.Fatal("expected detail pane visible in split mode")
	}

	prevOffset := m.listPanel.list.Offset()

	next, _ = m.Update(tea.MouseClickMsg{X: col + 1, Y: row, Button: tea.MouseLeft})
	m = next.(Model)

	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after click inside detail bounds")
	}

	next, _ = m.Update(tea.MouseWheelMsg{X: col + 1, Y: row, Button: tea.MouseWheelDown})
	m = next.(Model)

	if m.listPanel.list.Offset() != prevOffset {
		t.Fatalf("expected wheel event to leave the log list's scroll offset untouched (routed to detail instead), got offset=%d want %d", m.listPanel.list.Offset(), prevOffset)
	}
}

// TestWheelStillScrollsListWhenDetailNotFocused is the control for the above:
// without a click moving focus to the detail pane, a wheel event over the
// same coordinates still scrolls the log list as before.
func TestWheelStillScrollsListWhenDetailNotFocused(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows(commitRows(30))

	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = next.(Model)

	m.split, _ = m.split.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m.withSyncedListSize()
	m = m.withSyncedDetailSize()
	m.split = m.split.SetDetailFocused(false)

	col, row, _ := m.split.DetailOrigin()
	prevOffset := m.listPanel.list.Offset()

	next, _ = m.Update(tea.MouseWheelMsg{X: col + 1, Y: row, Button: tea.MouseWheelDown})
	m = next.(Model)

	if m.listPanel.list.Offset() == prevOffset {
		t.Fatal("expected the log list to still scroll on wheel events when the detail pane isn't focused")
	}
}

// TestMouseWheelWhileHelpOpenScrollsHelpNotList is a regression test for
// mouse-wheel events being swallowed while the help modal is open instead of
// scrolling its content.
func TestMouseWheelWhileHelpOpenScrollsHelpNotList(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.listPanel = m.listPanel.WithRows(commitRows(30))

	next, _ := m.Update(tea.WindowSizeMsg{Width: 64, Height: 20})
	m = next.(Model)
	m.help.Open(m.width, m.height)

	prevListOffset := m.listPanel.list.Offset()
	prevHelpOffset := m.help.Viewport.YOffset()

	next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = next.(Model)

	if m.listPanel.list.Offset() != prevListOffset {
		t.Fatalf("expected the log list not to scroll while help is open, before=%d after=%d", prevListOffset, m.listPanel.list.Offset())
	}
	if m.help.Viewport.YOffset() <= prevHelpOffset {
		t.Fatalf("expected help viewport to scroll on wheel while open, before=%d after=%d", prevHelpOffset, m.help.Viewport.YOffset())
	}
}
