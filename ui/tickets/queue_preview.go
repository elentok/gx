package tickets

import (
	"github.com/elentok/gx/ui"
)

// selectedQueueRow returns the queue row at m.selected, mirroring
// Model.selectedRow for the Tickets tab's sidebar. false means the queue is
// empty (nothing checked yet, or every checked ticket already cleared).
func (m QueueModel) selectedQueueRow() (queueRow, bool) {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return queueRow{}, false
	}
	return rows[m.selected], true
}

// queuePreviewContent builds the Queue tab's preview pane body for the
// currently selected row, via the same renderTicketPreview the Tickets tab
// uses (preview.go) - so both tabs' previews render identically. Nothing
// selected falls back to the same placeholder as the Tickets tab.
func (m QueueModel) queuePreviewContent(width int) string {
	row, ok := m.selectedQueueRow()
	if !ok {
		return ui.StyleDim.Render("  no ticket selected")
	}
	return renderTicketPreview(row.epic, row.ticket, width)
}

// queuePreviewSelectionKey identifies which row the preview is currently
// showing, mirroring Model.previewSelectionKey — used by
// syncQueuePreviewViewport to tell "still previewing the same row" from
// "selection moved" so it only resets scroll on the latter.
func (m QueueModel) queuePreviewSelectionKey() string {
	row, ok := m.selectedQueueRow()
	if !ok {
		return ""
	}
	return row.ticket.Path
}

// syncQueuePreviewViewport keeps the shared previewFocus's viewport size and
// content aligned with the current layout/selection (ticket 11), mirroring
// Model.syncPreviewViewport (model_preview_focus.go) — called after every
// Update so the Queue tab's preview gets real scroll/search instead of the
// old truncate-only rendering, even though (unlike Tickets) this tab has no
// focus-toggle into it yet (ticket 12).
func (m *QueueModel) syncQueuePreviewViewport() {
	if !m.ready {
		return
	}
	height := max(m.height-1, 1)
	_, previewW := splitPanelWidth(m.width)
	_, previewH := splitPanelHeight(m.width, height)
	width, ht := previewInnerSize(previewW, previewH)
	contentW := max(width-previewScrollbarGutter, 1)
	m.previewFocus.Sync(contentW, ht, m.queuePreviewSelectionKey(), m.queuePreviewContent)
}
