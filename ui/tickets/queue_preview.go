package tickets

import (
	"strings"

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

// queuePreviewLines renders queuePreviewContent clipped to a previewW x
// previewH panel: previewInnerSize accounts for the panel's own
// padding/header row (shared with the Tickets tab's preview sizing, see
// preview.go), and the result is truncated to the panel's visible height -
// the Queue tab's preview has no independent scroll of its own (ticket 15
// scopes it to "shows a preview pane", not scroll/search parity with the
// Tickets tab).
func (m QueueModel) queuePreviewLines(previewW, previewH int) []string {
	width, height := previewInnerSize(previewW, previewH)
	content := m.queuePreviewContent(width)
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}
