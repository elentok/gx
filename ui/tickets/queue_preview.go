package tickets

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tickets/queue"
)

// selectedQueueRow returns the queue row under m.rows.queueTree's current
// selection, mirroring Model.selectedRow for the Tickets tab's sidebar.
// false means nothing is selected — the queue is empty (nothing checked
// yet, or every checked ticket already cleared), or the selected entry is a
// header/separator/error row, a deliberate minor behavior change from the
// old rows()-only selection (05's "Real gap" section): those rows are left
// selectable but read as "nothing selected" to the preview.
func (m QueueModel) selectedQueueRow() (queue.Row, bool) {
	entries := m.rows.Tree.Entries()
	idx := m.rows.Tree.SelectedIndex()
	if idx < 0 || idx >= len(entries) {
		return queue.Row{}, false
	}
	entry := entries[idx]
	if entry.Value.Kind != queue.NodeTicket {
		return queue.Row{}, false
	}
	return entry.Value.Ticket, true
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
	return preview.RenderTicket(row.Epic, row.Ticket, width)
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
	return row.Ticket.Path
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
	_, previewW := preview.SplitPanelWidth(m.width)
	_, previewH := preview.SplitPanelHeight(m.width, m.layout().contentHeight())
	width, ht := preview.InnerSize(previewW, previewH)
	contentW := max(width-preview.ScrollbarGutter, 1)
	m.preview.Sync(contentW, ht, m.queuePreviewSelectionKey(), m.queuePreviewContentKey(), m.queuePreviewContent)
}

// queuePreviewContentKey is everything queuePreviewContent's output depends
// on besides width: the ticket's file mtime and body length (the file is the
// source of truth for a server-mode ticket, whose snapshot carries no body)
// and its rendered status. Empty (uncached) when nothing is selected.
func (m QueueModel) queuePreviewContentKey() string {
	row, ok := m.selectedQueueRow()
	if !ok {
		return ""
	}
	t := row.Ticket
	var mtime int64
	if info, err := os.Stat(t.FilePath()); err == nil {
		mtime = info.ModTime().UnixNano()
	}
	return fmt.Sprintf("%s|%d|%d|%d|%v", t.Path, mtime, len(t.Body), len(t.ReadErr), row.Epic.RenderedStatus(t))
}

// handleQueuePreviewKey processes key input while the Queue tab's preview
// panel has focus, mirroring Model.handlePreviewKey (model_preview_focus.go):
// its own search overlay, "h"/"left"/"esc" handing focus back to the list,
// "b" jumping to the bottom, and everything else delegated to the viewport's
// own scrolling.
func (m QueueModel) handleQueuePreviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cmd, res := m.preview.Update(msg)
	if res == preview.Back {
		return m, nav.Back()
	}
	return m, cmd
}
