package tickets

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ui"
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
	// previewHidden backs the "tv" chord. Kept per tab, in memory only.
	previewHidden bool

	cache              previewCache
	contentHighlighted bool // whether the viewport currently holds search-highlighted content
}

// previewCache is Sync's last contentFor result, with the inputs it was built from.
type previewCache struct {
	key       string
	width     int
	content   string
	target    int
	hasTarget bool
	valid     bool
}

// togglePreview flips previewHidden. Hiding hands focus back to the list,
// since a hidden panel can't receive keys.
func (p *previewFocus) togglePreview() {
	p.previewHidden = !p.previewHidden
	if p.previewHidden {
		p.focus = focusSidebar
	}
}

// focusPreviewPane is the one way to give the preview focus, a no-op while
// it's hidden.
func (p *previewFocus) focusPreviewPane() {
	if !p.previewHidden {
		p.focus = focusPreview
	}
}

// panelWidths/panelHeights wrap splitPanelWidth/splitPanelHeight, giving the
// list the whole area while the preview is hidden.
func (p previewFocus) panelWidths(width int) (sidebarW, previewW int) {
	if p.previewHidden {
		return width, 0
	}
	return splitPanelWidth(width)
}

func (p previewFocus) panelHeights(width, total int) (sidebarH, previewH int) {
	if p.previewHidden {
		return total, 0
	}
	return splitPanelHeight(width, total)
}

// joinPanels lays the list and preview out side by side (or stacked on
// narrow terminals) with a seam between them, or the list alone while the
// preview is hidden.
func (p previewFocus) joinPanels(width, sidebarW, sidebarH int, sidebarView, previewView string) string {
	switch {
	case p.previewHidden:
		return sidebarView
	case useStackedLayout(width):
		return lipgloss.JoinVertical(lipgloss.Left, sidebarView, ui.RenderSeamRow(sidebarW, ui.SeamColor), previewView)
	default:
		return lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, ui.RenderSeamColumn(sidebarH, ui.SeamColor), previewView)
	}
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
// on the previous call — not on every resize/refresh. contentFor's second
// and third return values are an optional scroll-target line index (e.g. a
// parked ticket's park section — see renderTicketPreview): when the
// selection just changed and a target is reported, the viewport centers on
// it instead of the usual GotoTop.
//
// contentKey names everything contentFor's output depends on besides width
// (a ticket's file mtime, body length, status...). While it and width are
// unchanged, contentFor is skipped and the cached output reused — Sync runs on
// every Update, and a render is far costlier than building the key. An empty
// contentKey disables the cache.
func (p *previewFocus) Sync(width, height int, selKey, contentKey string, contentFor func(width int) (content string, scrollTarget int, hasScrollTarget bool)) {
	p.previewVP.SetWidth(width)
	p.previewVP.SetHeight(height)

	selectionChanged := selKey != p.previewSelKey
	p.previewSelKey = selKey

	cacheHit := contentKey != "" && p.cache.valid && p.cache.key == contentKey && p.cache.width == width
	if !cacheHit {
		content, target, hasTarget := contentFor(width)
		p.cache = previewCache{key: contentKey, width: width, content: content, target: target, hasTarget: hasTarget, valid: contentKey != ""}
	}
	content, target, hasTarget := p.cache.content, p.cache.target, p.cache.hasTarget

	highlighted := p.previewSearch.HasQuery()
	if highlighted {
		content = highlightPreviewContent(content, p.previewSearch)
	}
	// Re-setting identical plain content is the per-Update cost the cache exists to avoid.
	if !cacheHit || highlighted || p.contentHighlighted {
		p.previewVP.SetContent(content)
	}
	p.contentHighlighted = highlighted

	if selectionChanged {
		if hasTarget {
			offset := target - p.previewVP.Height()/2
			p.previewVP.SetYOffset(max(offset, 0))
		} else {
			p.previewVP.GotoTop()
		}
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

// previewRect returns the preview panel's absolute on-screen bounds for a
// tab of the given width/contentHeight, computed from the same
// panelWidths/panelHeights/useStackedLayout math each tab's View uses to lay
// out its panels - so click hit-testing stays in sync with what's actually
// rendered, in both tabs at once. A hidden preview has an empty rect.
func (p previewFocus) previewRect(width, contentHeight int) (x, y, w, h int) {
	if p.previewHidden {
		return 0, 0, 0, 0
	}
	sidebarW, previewW := splitPanelWidth(width)
	sidebarH, previewH := splitPanelHeight(width, contentHeight)
	if useStackedLayout(width) {
		return 0, sidebarH + 1, previewW, previewH
	}
	return sidebarW + 1, 0, previewW, previewH
}

// inPreview bounds-checks a mouse click against the preview panel's rect (as
// returned by previewRect for the given width/contentHeight), so the caller's
// row-under-click logic can skip a click that landed on the preview.
func (p *previewFocus) inPreview(mouse tea.Mouse, width, contentHeight int) bool {
	px, py, pw, ph := p.previewRect(width, contentHeight)
	return mouse.X >= px && mouse.X < px+pw && mouse.Y >= py && mouse.Y < py+ph
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
