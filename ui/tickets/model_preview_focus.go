package tickets

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/tickets/preview"
)

// previewSelectionKey identifies which row's content the preview is
// currently showing, used by syncPreviewViewport to tell "still previewing
// the same row" (keep the scroll position) from "selection moved" (reset it
// to the top).
func (m Model) previewSelectionKey() string {
	r, ok := m.selectedRow()
	if !ok {
		return ""
	}
	if r.isEpic() {
		return fmt.Sprintf("epic:%d", r.epicIdx)
	}
	return fmt.Sprintf("ticket:%d:%d", r.epicIdx, r.ticketIdx)
}

// syncPreviewViewport keeps m.previewVP's size and content aligned with the
// current layout/selection, called after every Update (see Update's
// wrapper): resizing it to the preview panel's current inner dimensions,
// refreshing its content from the selected row, and resetting scroll only
// when the selected row itself changed (not on every resize/refresh).
func (m *Model) syncPreviewViewport() {
	if !m.ready {
		return
	}
	_, previewW := m.splitWidth()
	_, previewH := m.splitHeight(m.contentHeight())
	width, height := preview.InnerSize(previewW, previewH)
	contentW := max(width-preview.ScrollbarGutter, 1)

	m.preview.Sync(contentW, height, m.previewSelectionKey(), "", m.previewContent)
}

// previewContent builds the selected row's preview. Nothing selected (e.g.
// an empty `.scratch/`) falls back to the tab's empty-preview placeholder. A
// ticket row is rendered by renderTicketPreview, shared with the Queue tab's
// own preview pane (see queue_preview.go). An epic row gets its own header
// (name + optional [map] badge + open/total count) followed by its ticket.md
// body. The second and third return values are the pause-section scroll
// target line index and whether one exists — always false for an epic row.
func (m Model) previewContent(width int) (string, int, bool) {
	r, ok := m.selectedRow()
	if !ok {
		return ui.StyleDim.Render("  no ticket selected"), 0, false
	}
	if r.isEpic() {
		return preview.EpicContent(preview.WithEpicTicketMD(m.epicAt(r)), width), 0, false
	}

	epic := m.epicAt(r)
	t := epic.Tickets[r.ticketIdx]
	return preview.RenderTicket(epic, t, width)
}

// handlePreviewKey routes a key to the preview pane while it has focus.
func (m Model) handlePreviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cmd, res := m.preview.Update(msg)
	if res == preview.Back {
		return m, nav.Back()
	}
	return m, cmd
}
