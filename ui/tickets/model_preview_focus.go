package tickets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
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

	m.previewVP.SetContent(m.previewContent(contentW))

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
