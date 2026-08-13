package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
)

// selectedQueueRow returns the queue row under m.queueTree's current
// selection, mirroring Model.selectedRow for the Tickets tab's sidebar.
// false means nothing is selected — the queue is empty (nothing checked
// yet, or every checked ticket already cleared), or the selected entry is a
// header/separator/error row, a deliberate minor behavior change from the
// old rows()-only selection (05's "Real gap" section): those rows are left
// selectable but read as "nothing selected" to the preview.
func (m QueueModel) selectedQueueRow() (queueRow, bool) {
	entries := m.queueTree.Entries()
	idx := m.queueTree.SelectedIndex()
	if idx < 0 || idx >= len(entries) {
		return queueRow{}, false
	}
	entry := entries[idx]
	if entry.Value.kind != nodeQueueTicket {
		return queueRow{}, false
	}
	return entry.Value.ticket, true
}

// queuePreviewContent builds the Queue tab's preview pane body for the
// currently selected row, via the same renderTicketPreview the Tickets tab
// uses (preview.go) - so both tabs' previews render identically. Nothing
// selected falls back to the same placeholder as the Tickets tab.
func (m QueueModel) queuePreviewContent(width int) (string, int, bool) {
	row, ok := m.selectedQueueRow()
	if !ok {
		return ui.StyleDim.Render("  no ticket selected"), 0, false
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
// old truncate-only rendering.
func (m *QueueModel) syncQueuePreviewViewport() {
	if !m.ready {
		return
	}
	_, previewW := splitPanelWidth(m.width)
	_, previewH := splitPanelHeight(m.width, m.contentHeight())
	width, ht := previewInnerSize(previewW, previewH)
	contentW := max(width-previewScrollbarGutter, 1)
	m.previewFocus.Sync(contentW, ht, m.queuePreviewSelectionKey(), m.queuePreviewContent)
}

// handleQueuePreviewKey processes key input while the Queue tab's preview
// panel has focus, mirroring Model.handlePreviewKey (model_preview_focus.go):
// its own search overlay, "h"/"left"/"esc" handing focus back to the list,
// "b" jumping to the bottom, and everything else delegated to the viewport's
// own scrolling.
func (m QueueModel) handleQueuePreviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if handled, cmd := m.previewFocus.updatePreviewKey(msg); handled {
		return m, cmd
	}

	if msg.String() == "q" {
		return m, nav.Back()
	}

	switch msg.String() {
	case "h", "left", "esc":
		m.focus = focusSidebar
		return m, nil
	case "b":
		m.previewVP.GotoBottom()
		return m, nil
	}

	var cmd tea.Cmd
	m.previewVP, cmd = m.previewVP.Update(msg)
	return m, cmd
}
