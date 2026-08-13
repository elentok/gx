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

	m.previewFocus.Sync(contentW, height, m.previewSelectionKey(), m.previewContent)
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

// handlePreviewKey processes key input while the preview panel has focus:
// its own search overlay, "h"/"left"/"esc" handing focus back to the
// sidebar, "b" jumping straight to the bottom (overriding the viewport's own
// "b" default of a page-up — see bubbles/viewport's DefaultKeyMap), and
// everything else delegated to the viewport's own scrolling (j/k, up/down,
// pgup/pgdn, ctrl+u/d, etc.).
func (m Model) handlePreviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
