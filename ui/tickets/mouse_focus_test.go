package tickets

import (
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
