package tickets

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui/search"
)

// This file holds the preview panel's search/scroll logic shared between
// Model (ui/tickets' epic-tree sidebar) and FlatModel (the flat ralph-loop
// TUI, flat.go): both pair a viewport.Model with a search.Model over
// glamour-rendered ticket body content, and none of this logic touches
// either model's own tree-vs-flat selection shape — see model_preview_focus.go
// and flat_preview.go for what stays per-model (h/left/esc focus handoff,
// "q" quit-vs-nav.Back, and previewContent's differing ticket/epic cases).

// previewSearchMatchStatus formats a preview search's current match
// position, shown as its panel's right-aligned header text.
func previewSearchMatchStatus(s search.Model) string {
	if s.HasQuery() && s.MatchesCount() > 0 {
		return fmt.Sprintf("%d/%d matches", s.Cursor()+1, s.MatchesCount())
	}
	return ""
}

// recomputePreviewSearchMatches rebuilds a preview search's match set
// against the viewport's current (already glamour-rendered) content lines:
// case-insensitive substring over each line's plain text (ANSI stripped).
// DataIndex doubles as the line index — the preview has no separate
// "viewport row" concept the way the sidebar's row-based search does.
func recomputePreviewSearchMatches(vp viewport.Model, s *search.Model) {
	q := strings.ToLower(strings.TrimSpace(s.Query()))
	if q == "" {
		s.SetMatches(nil)
		return
	}
	lines := strings.Split(vp.GetContent(), "\n")
	matches := make([]search.Match, 0)
	for i, line := range lines {
		if strings.Contains(strings.ToLower(ansi.Strip(line)), q) {
			matches = append(matches, search.Match{DataIndex: i})
		}
	}
	s.SetMatches(matches)
}

// highlightPreviewContent wraps each search match in content (as built by
// previewContent, before it's handed to the viewport) in the
// search-highlight style. This must run on previewContent's own
// word/run-level ANSI output, not on the viewport's further-processed
// per-cell output — ansi.Cut mishandles heavily fragmented per-character
// runs, corrupting the escape stream (see the preview-search-highlight fix).
func highlightPreviewContent(content string, s search.Model) string {
	lines := strings.Split(content, "\n")
	query := s.Query()
	for i, line := range lines {
		if matched, current := previewSearchMatch(s, i); matched {
			lines[i] = highlightPreviewLine(line, query, current)
		}
	}
	return strings.Join(lines, "\n")
}

// previewSearchMatch reports whether the preview content's line at absIdx
// (its index into the viewport's GetContent(), not the currently visible
// window) is a search match, and whether it's the match under the search
// cursor (n/N target).
func previewSearchMatch(s search.Model, absIdx int) (matched, current bool) {
	pos, ok := s.MatchPosByDataIndex(absIdx)
	if !ok {
		return false, false
	}
	return true, pos == s.Cursor()
}

// jumpToCurrentPreviewMatch scrolls the preview viewport so the search
// cursor's current match line is visible, centering it when the viewport is
// tall enough to make that meaningful.
func jumpToCurrentPreviewMatch(vp *viewport.Model, s search.Model) {
	match, ok := s.Match(s.Cursor())
	if !ok {
		return
	}
	offset := match.DataIndex - vp.Height()/2
	vp.SetYOffset(max(offset, 0))
}

// updatePreviewSearchKey routes a key through the preview's own search
// overlay if it's mid-input/navigating results, recomputing matches and
// re-centering the viewport as needed. Returns handled=false when the
// search model didn't consume msg, so the caller falls through to its own
// h/left/esc focus handoff and the viewport's own scroll bindings.
func updatePreviewSearchKey(msg tea.KeyPressMsg, vp *viewport.Model, s *search.Model) (handled bool, cmd tea.Cmd) {
	nextSearch, cmd, result := s.Update(msg)
	if !result.Handled {
		return false, nil
	}
	*s = nextSearch
	if result.QueryChanged {
		recomputePreviewSearchMatches(*vp, s)
	}
	if result.QueryChanged || result.CursorChanged {
		jumpToCurrentPreviewMatch(vp, *s)
	}
	return true, cmd
}
