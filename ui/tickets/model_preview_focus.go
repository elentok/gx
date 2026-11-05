package tickets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
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
	width, height := previewInnerSize(previewW, previewH)
	contentW := max(width-previewScrollbarGutter, 1)

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
		return previewEpicContent(withEpicTicketMD(m.epicAt(r)), width), 0, false
	}

	epic := m.epicAt(r)
	t := epic.Tickets[r.ticketIdx]
	return renderTicketPreview(epic, t, width)
}

// highlightPreviewLine wraps query's first match on an already
// glamour-rendered (ANSI-styled) line in the search-highlight style. It
// mirrors search.Highlight's byte-offset matching, but walks ANSI sequences
// before rebuilding the line so it never splits an escape sequence. ANSI
// styling inside the matched run is deliberately replaced by the overlay;
// preserving it would let its resets cancel the search highlight.
func highlightPreviewLine(line, query string, current bool) string {
	plain := ansi.Strip(line)
	lower := strings.ToLower(plain)
	lq := strings.ToLower(query)
	idx := strings.Index(lower, lq)
	if idx < 0 {
		return line
	}
	end := min(idx+len(query), len(plain))

	style := ui.StyleSearchResult
	if current {
		style = ui.StyleActiveSearchResult
	}

	var prefix, matched, suffix strings.Builder
	state := ansi.NormalState
	plainOffset := 0
	matchStarted := false
	matchEnded := false
	for rest := line; len(rest) > 0; {
		seq, _, n, nextState := ansi.DecodeSequence(rest, state, nil)
		if n == 0 {
			// DecodeSequence only returns zero for malformed input. Copy its
			// remaining bytes unchanged rather than risking an infinite loop.
			suffix.WriteString(rest)
			break
		}
		state = nextState
		rest = rest[n:]

		plainSeq := ansi.Strip(seq)
		if plainSeq == "" {
			switch {
			case !matchStarted:
				prefix.WriteString(seq)
			case matchEnded:
				suffix.WriteString(seq)
			}
			continue
		}

		seqStart := plainOffset
		plainOffset += len(plainSeq)
		if seqStart < end && plainOffset > idx {
			matchStarted = true
			matched.WriteString(plainSeq)
			continue
		}
		if matchStarted {
			matchEnded = true
		}
		if plainOffset <= idx {
			prefix.WriteString(seq)
		} else {
			suffix.WriteString(seq)
		}
	}

	return prefix.String() + style.Render(matched.String()) + suffix.String()
}

// handlePreviewKey routes a key to the preview pane while it has focus.
func (m Model) handlePreviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cmd, res := m.preview.update(msg)
	if res == previewBack {
		return m, nav.Back()
	}
	return m, cmd
}
