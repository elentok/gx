package tickets

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// TestModel_ClickInsidePreviewBoundsFocusesItAndRoutesWheel covers the
// preview-bounds hit-test: a click landing inside previewVP's rendered
// bounds shifts focus to it (mirroring "l"/"enter"), and a subsequent wheel
// event then scrolls the preview instead of the sidebar.
func TestModel_ClickInsidePreviewBoundsFocusesItAndRoutesWheel(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\n"+strings.Repeat("Line of body text.\n\n", 100))

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// rows: [section header, epic, ticket] - move down twice to select the
	// long-bodied ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	if m.focus != focusSidebar {
		t.Fatalf("expected initial focus on sidebar, got focus=%v", m.focus)
	}

	px, py, _, _ := m.previewRect()
	updated, _ = m.Update(tea.MouseClickMsg{X: px + 1, Y: py + 1, Button: tea.MouseLeft})
	m = updated.(Model)

	if m.focus != focusPreview {
		t.Fatalf("expected preview focus after click inside preview bounds, got focus=%v", m.focus)
	}

	initialOffset := m.previewVP.YOffset()
	updated, _ = m.Update(tea.MouseWheelMsg{X: px + 1, Y: py + 1, Button: tea.MouseWheelDown})
	m = updated.(Model)

	if m.sidebarTree.ScrollOffset() != 0 {
		t.Fatalf("expected sidebar scroll offset to stay at 0, got %d", m.sidebarTree.ScrollOffset())
	}
	if m.previewVP.YOffset() == initialOffset {
		t.Fatalf("expected preview viewport to scroll on wheel event while focused, offset stayed at %d", initialOffset)
	}
}

// TestModel_ClickInsideSidebarRestoresFocus covers the reverse direction: once
// the preview has focus, clicking back inside the sidebar hands focus back to
// it, so wheel events resume scrolling the sidebar rather than the preview.
func TestModel_ClickInsideSidebarRestoresFocus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.focus = focusPreview

	line := selectedSidebarLine(t, m)
	updated, _ = m.Update(tea.MouseClickMsg{X: 1, Y: line + 1, Button: tea.MouseLeft})
	m = updated.(Model)

	if m.focus != focusSidebar {
		t.Fatalf("expected sidebar focus after click inside sidebar bounds, got focus=%v", m.focus)
	}
}

// TestModel_WheelOverSidebarScrollsItRegardlessOfFocus pins keyboard focus to
// the preview pane, then sends a wheel event at coordinates inside the
// sidebar's rect — proving routing follows the cursor, not leftover focus
// state (a still-click-gated implementation would instead scroll the
// preview, or no-op).
func TestModel_WheelOverSidebarScrollsItRegardlessOfFocus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 1; i <= 60; i++ {
		writeTicket(t, root, "epic", fmt.Sprintf("%02d-ticket.md", i), "Status: open\n\nBody.\n")
	}

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m = updated.(Model)

	m.focus = focusPreview
	previewOffsetBefore := m.previewVP.YOffset()
	sidebarOffsetBefore := m.sidebarTree.ScrollOffset()

	sx, sy, _, _ := m.sidebarRect()
	updated, _ = m.Update(tea.MouseWheelMsg{X: sx + 1, Y: sy + 1, Button: tea.MouseWheelDown})
	m = updated.(Model)

	if m.sidebarTree.ScrollOffset() <= sidebarOffsetBefore {
		t.Fatalf("expected sidebar scroll offset to increase from %d, got %d", sidebarOffsetBefore, m.sidebarTree.ScrollOffset())
	}
	if m.previewVP.YOffset() != previewOffsetBefore {
		t.Fatalf("expected preview scroll offset to stay at %d, got %d", previewOffsetBefore, m.previewVP.YOffset())
	}
	if m.focus != focusPreview {
		t.Fatalf("expected focus to remain on preview, got focus=%v", m.focus)
	}
}

// TestModel_WheelOverPreviewScrollsItRegardlessOfFocus is the mirror of
// TestModel_WheelOverSidebarScrollsItRegardlessOfFocus: focus pinned to the
// sidebar, wheel event over the preview's rect scrolls the preview.
func TestModel_WheelOverPreviewScrollsItRegardlessOfFocus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\n"+strings.Repeat("Line of body text.\n\n", 100))

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// rows: [section header, epic, ticket] - move down twice to select the
	// long-bodied ticket.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	m.focus = focusSidebar
	previewOffsetBefore := m.previewVP.YOffset()
	sidebarOffsetBefore := m.sidebarTree.ScrollOffset()

	px, py, _, _ := m.previewRect()
	updated, _ = m.Update(tea.MouseWheelMsg{X: px + 1, Y: py + 1, Button: tea.MouseWheelDown})
	m = updated.(Model)

	if m.previewVP.YOffset() <= previewOffsetBefore {
		t.Fatalf("expected preview scroll offset to increase from %d, got %d", previewOffsetBefore, m.previewVP.YOffset())
	}
	if m.sidebarTree.ScrollOffset() != sidebarOffsetBefore {
		t.Fatalf("expected sidebar scroll offset to stay at %d, got %d", sidebarOffsetBefore, m.sidebarTree.ScrollOffset())
	}
	if m.focus != focusSidebar {
		t.Fatalf("expected focus to remain on sidebar, got focus=%v", m.focus)
	}
}

// TestModel_WheelOverPaneWithNoOverflowIsNoop covers a pane with nothing
// left to scroll: the wheel event no-ops rather than falling back to
// whichever pane holds keyboard focus.
func TestModel_WheelOverPaneWithNoOverflowIsNoop(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-only-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.focus = focusPreview
	previewOffsetBefore := m.previewVP.YOffset()
	sidebarOffsetBefore := m.sidebarTree.ScrollOffset()

	sx, sy, _, _ := m.sidebarRect()
	updated, _ = m.Update(tea.MouseWheelMsg{X: sx + 1, Y: sy + 1, Button: tea.MouseWheelDown})
	m = updated.(Model)

	if m.sidebarTree.ScrollOffset() != sidebarOffsetBefore {
		t.Fatalf("expected sidebar scroll offset to stay at %d, got %d", sidebarOffsetBefore, m.sidebarTree.ScrollOffset())
	}
	if m.previewVP.YOffset() != previewOffsetBefore {
		t.Fatalf("expected preview scroll offset to stay at %d, got %d", previewOffsetBefore, m.previewVP.YOffset())
	}
}

// TestModel_MouseWheelWhileHelpOpenScrollsHelpNotPreview covers the missing
// help.IsOpen guard on the top-level tea.MouseWheelMsg case: while the help
// modal is open, wheel events must scroll it instead of reaching the
// sidebar/preview panes behind it.
func TestModel_MouseWheelWhileHelpOpenScrollsHelpNotPreview(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\n"+strings.Repeat("Line of body text.\n\n", 100))

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	px, py, _, _ := m.previewRect()
	updated, _ = m.Update(tea.MouseClickMsg{X: px + 1, Y: py + 1, Button: tea.MouseLeft})
	m = updated.(Model)

	m.help.Open(m.width, m.height)

	prevPreviewOffset := m.previewVP.YOffset()
	prevSidebarOffset := m.sidebarTree.ScrollOffset()
	prevHelpOffset := m.help.Viewport.YOffset()

	updated, _ = m.Update(tea.MouseWheelMsg{X: px + 1, Y: py + 1, Button: tea.MouseWheelDown})
	m = updated.(Model)

	if m.previewVP.YOffset() != prevPreviewOffset {
		t.Fatalf("expected preview not to scroll while help is open, before=%d after=%d", prevPreviewOffset, m.previewVP.YOffset())
	}
	if m.sidebarTree.ScrollOffset() != prevSidebarOffset {
		t.Fatalf("expected sidebar not to scroll while help is open, before=%d after=%d", prevSidebarOffset, m.sidebarTree.ScrollOffset())
	}
	if m.help.Viewport.YOffset() <= prevHelpOffset {
		t.Fatalf("expected help viewport to scroll on wheel while open, before=%d after=%d", prevHelpOffset, m.help.Viewport.YOffset())
	}
}
