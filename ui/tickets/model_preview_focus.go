package tickets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/search"
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
	h := m.contentHeight()
	width, height := m.previewInnerSize(previewW, h)
	contentW := max(width-previewScrollbarGutter, 1)

	m.previewVP.SetWidth(contentW)
	m.previewVP.SetHeight(height)

	key := m.previewSelectionKey()
	selectionChanged := key != m.previewSelKey
	m.previewSelKey = key

	content := m.previewContent(contentW)
	if m.previewSearch.HasQuery() {
		content = m.highlightPreviewContent(content)
	}
	m.previewVP.SetContent(content)

	if selectionChanged {
		m.previewVP.GotoTop()
		m.previewSearch.SetMatches(nil)
	}
}

// previewMatchStatus mirrors searchMatchStatus for the preview panel's own
// search, shown as its panel's right-aligned header text.
func (m Model) previewMatchStatus() string {
	if m.previewSearch.HasQuery() && m.previewSearch.MatchesCount() > 0 {
		return fmt.Sprintf("%d/%d matches", m.previewSearch.Cursor()+1, m.previewSearch.MatchesCount())
	}
	return ""
}

// recomputePreviewSearchMatches rebuilds the preview search's match set
// against the viewport's current (already glamour-rendered) content lines:
// case-insensitive substring over each line's plain text (ANSI stripped).
// DataIndex doubles as the line index — the preview has no separate
// "viewport row" concept the way the sidebar's row-based search does.
func (m *Model) recomputePreviewSearchMatches() {
	q := strings.ToLower(strings.TrimSpace(m.previewSearch.Query()))
	if q == "" {
		m.previewSearch.SetMatches(nil)
		return
	}
	lines := strings.Split(m.previewVP.GetContent(), "\n")
	matches := make([]search.Match, 0)
	for i, line := range lines {
		if strings.Contains(strings.ToLower(ansi.Strip(line)), q) {
			matches = append(matches, search.Match{DataIndex: i})
		}
	}
	m.previewSearch.SetMatches(matches)
}

// highlightPreviewContent wraps each search match in content (as built by
// previewContent, before it's handed to the viewport) in the
// search-highlight style. This must run on previewContent's own
// word/run-level ANSI output, not on m.previewVP.View()'s further-processed
// per-cell output — ansi.Cut mishandles heavily fragmented per-character
// runs, corrupting the escape stream (see the preview-search-highlight fix).
func (m Model) highlightPreviewContent(content string) string {
	lines := strings.Split(content, "\n")
	query := m.previewSearch.Query()
	for i, line := range lines {
		if matched, current := m.previewSearchMatch(i); matched {
			lines[i] = highlightPreviewLine(line, query, current)
		}
	}
	return strings.Join(lines, "\n")
}

// previewSearchMatch reports whether the preview content's line at absIdx
// (its index into m.previewVP.GetContent(), not the currently visible
// window) is a search match, and whether it's the match under the search
// cursor (n/N target) — mirrors searchMatch's sidebar equivalent.
func (m Model) previewSearchMatch(absIdx int) (matched, current bool) {
	pos, ok := m.previewSearch.MatchPosByDataIndex(absIdx)
	if !ok {
		return false, false
	}
	return true, pos == m.previewSearch.Cursor()
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

// jumpToCurrentPreviewMatch scrolls the preview viewport so the search
// cursor's current match line is visible, centering it when the viewport is
// tall enough to make that meaningful.
func (m *Model) jumpToCurrentPreviewMatch() {
	match, ok := m.previewSearch.Match(m.previewSearch.Cursor())
	if !ok {
		return
	}
	offset := match.DataIndex - m.previewVP.Height()/2
	m.previewVP.SetYOffset(max(offset, 0))
}

// focusPreviewOrExpand implements "l"/"enter": on a ticket row, or on an
// epic row that's already expanded, it hands focus to the preview panel to
// scroll/search its body. On a collapsed epic row it instead reports false
// so the caller falls back to expanding it — the first enter/l on a
// collapsed epic expands it, and only a second press (now that it's
// expanded) moves focus to the preview.
func (m *Model) focusPreviewOrExpand() bool {
	r, ok := m.selectedRow()
	if !ok {
		return false
	}
	if r.isEpic() && m.isCollapsed(m.epics[r.epicIdx]) {
		return false
	}
	m.focus = focusPreview
	return true
}

// handlePreviewKey processes key input while the preview panel has focus:
// its own search overlay, "h"/"left"/"esc" handing focus back to the
// sidebar, and everything else delegated to the viewport's own scrolling
// (j/k, up/down, pgup/pgdn, ctrl+u/d, etc. — see bubbles/viewport's
// DefaultKeyMap).
func (m Model) handlePreviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if nextSearch, cmd, result := m.previewSearch.Update(msg); result.Handled {
		m.previewSearch = nextSearch
		if result.QueryChanged {
			m.recomputePreviewSearchMatches()
		}
		if result.QueryChanged || result.CursorChanged {
			m.jumpToCurrentPreviewMatch()
		}
		return m, cmd
	}

	if msg.String() == "q" {
		return m, nav.Back()
	}

	switch msg.String() {
	case "h", "left", "esc":
		m.focus = focusSidebar
		return m, nil
	}

	var cmd tea.Cmd
	m.previewVP, cmd = m.previewVP.Update(msg)
	return m, cmd
}
