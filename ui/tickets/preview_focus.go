package tickets

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/search"
)

// previewFocus is the sidebar+preview pairing's shared preview/viewport
// machinery (ADR 0009's worktrees archetype): a scrollable viewport over the
// selected row's rendered content, that viewport's own "/" search overlay,
// selKey to tell "still previewing the same row" (keep scroll position) from
// "selection moved" (reset it), and focus for which of the pairing's two
// panels currently receives key input. Held by Model (the Tickets tab, via
// embedding — see model.go) and, from ticket 11, QueueModel.
//
// Field names (previewVP, previewSearch, previewSelKey, focus) are promoted
// through embedding rather than accessed via a nested field, so existing
// call sites and tests keep reading/writing m.previewVP etc. directly.
type previewFocus struct {
	focus         focusPane
	previewVP     viewport.Model
	previewSelKey string // identifies the previewed row, to reset scroll on selection change
	previewSearch search.Model
}

func newPreviewFocus() previewFocus {
	return previewFocus{
		previewSearch: search.NewModel(),
		previewVP:     viewport.New(),
	}
}

// Sync resizes the viewport to width x height, refreshes its content via
// contentFor, and resets scroll position/search matches only when selKey
// (identifying which row is being previewed) differs from the value passed
// on the previous call — not on every resize/refresh.
func (p *previewFocus) Sync(width, height int, selKey string, contentFor func(width int) string) {
	p.previewVP.SetWidth(width)
	p.previewVP.SetHeight(height)

	selectionChanged := selKey != p.previewSelKey
	p.previewSelKey = selKey

	content := contentFor(width)
	if p.previewSearch.HasQuery() {
		content = highlightPreviewContent(content, p.previewSearch)
	}
	p.previewVP.SetContent(content)

	if selectionChanged {
		p.previewVP.GotoTop()
		p.previewSearch.SetMatches(nil)
	}
}

// previewMatchStatus formats the preview search's current match position,
// shown as its panel's right-aligned header text.
func (p previewFocus) previewMatchStatus() string {
	return previewSearchMatchStatus(p.previewSearch)
}

// previewLines renders the viewport's currently visible content paired with
// its scroll indicator.
func (p previewFocus) previewLines() []string {
	return renderViewportWithScrollbar(p.previewVP)
}

// updatePreviewKey routes a key through the preview's own search overlay,
// when it's mid-input or navigating results. handled is false otherwise, so
// the caller can interpret the key itself: a tab-specific quit key,
// "h"/"left"/"esc" to hand focus back to the sidebar, or fall through to the
// viewport's own scrolling (j/k, up/down, pgup/pgdn, ctrl+u/d, etc. — see
// bubbles/viewport's DefaultKeyMap).
func (p *previewFocus) updatePreviewKey(msg tea.KeyPressMsg) (handled bool, cmd tea.Cmd) {
	if handled, cmd := updatePreviewSearchKey(msg, &p.previewVP, &p.previewSearch); handled {
		return true, cmd
	}
	return false, nil
}
