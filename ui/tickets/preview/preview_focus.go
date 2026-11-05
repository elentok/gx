package preview

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
)

// FocusPane is which of the tickets tab's two panels currently receives key
// input: the sidebar (row navigation/collapse) or the preview (scroll/
// search over the selected row's rendered body).
type FocusPane int

const (
	FocusSidebar FocusPane = iota
	FocusPreview
)

// Pane is the sidebar+preview pairing's shared preview/viewport
// machinery (ADR 0009's worktrees archetype): a scrollable viewport over the
// selected row's rendered content, that viewport's own "/" search overlay,
// selKey to tell "still previewing the same row" (keep scroll position) from
// "selection moved" (reset it), and Focus for which of the pairing's two
// panels currently receives key input. Held as the named field `preview` by
// Model (the Tickets tab) and QueueModel.
//
// It takes plain inputs (sizes, a selection key, a content callback) and never
// reads a parent model; what a key meant for the parent comes back as a
// Result.
type Pane struct {
	Focus  FocusPane
	VP     viewport.Model
	selKey string // identifies the previewed row, to reset scroll on selection change
	Search search.Model
	// Hidden backs the "tv" chord. Kept per tab, in memory only.
	Hidden bool

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

// Toggle flips Hidden. Hiding hands focus back to the list,
// since a hidden panel can't receive keys.
func (p *Pane) Toggle() {
	p.Hidden = !p.Hidden
	if p.Hidden {
		p.Focus = FocusSidebar
	}
}

// FocusPreviewPane is the one way to give the preview focus, a no-op while
// it's hidden.
func (p *Pane) FocusPreviewPane() {
	if !p.Hidden {
		p.Focus = FocusPreview
	}
}

// UseStackedLayout, SplitPanelWidth and SplitPanelHeight are free functions
// so the Queue tab's own list+preview split shares the exact same layout math
// as the Tickets tab: both tabs' panels size identically at a given terminal
// width.
func UseStackedLayout(width int) bool {
	return width <= 100
}

func SplitPanelWidth(width int) (sidebarW, previewW int) {
	if UseStackedLayout(width) {
		return width, width
	}
	w := width - 1
	sidebarW = w / 2
	previewW = w - sidebarW
	return
}

func SplitPanelHeight(width, total int) (sidebarH, previewH int) {
	if !UseStackedLayout(width) {
		return total, total
	}
	total-- // seam row
	sidebarH = total / 2
	previewH = total - sidebarH
	return
}

// PanelWidths/PanelHeights wrap SplitPanelWidth/SplitPanelHeight, giving the
// list the whole area while the preview is hidden.
func (p Pane) PanelWidths(width int) (sidebarW, previewW int) {
	if p.Hidden {
		return width, 0
	}
	return SplitPanelWidth(width)
}

func (p Pane) PanelHeights(width, total int) (sidebarH, previewH int) {
	if p.Hidden {
		return total, 0
	}
	return SplitPanelHeight(width, total)
}

// JoinPanels lays the list and preview out side by side (or stacked on
// narrow terminals) with a seam between them, or the list alone while the
// preview is hidden.
func (p Pane) JoinPanels(width, sidebarW, sidebarH int, sidebarView, previewView string) string {
	switch {
	case p.Hidden:
		return sidebarView
	case UseStackedLayout(width):
		return lipgloss.JoinVertical(lipgloss.Left, sidebarView, ui.RenderSeamRow(sidebarW, ui.SeamColor), previewView)
	default:
		return lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, ui.RenderSeamColumn(sidebarH, ui.SeamColor), previewView)
	}
}

func NewPane() Pane {
	return Pane{
		Search: search.NewModel(),
		VP:     viewport.New(),
	}
}

// Sync resizes the viewport to width x height, refreshes its content via
// contentFor, and resets scroll position/search matches only when selKey
// (identifying which row is being previewed) differs from the value passed
// on the previous call — not on every resize/refresh. contentFor's second
// and third return values are an optional scroll-target line index (e.g. a
// paused ticket's pause section — see RenderTicket): when the
// selection just changed and a target is reported, the viewport centers on
// it instead of the usual GotoTop.
//
// contentKey names everything contentFor's output depends on besides width
// (a ticket's file mtime, body length, status...). While it and width are
// unchanged, contentFor is skipped and the cached output reused — Sync runs on
// every Update, and a render is far costlier than building the key. An empty
// contentKey disables the cache.
func (p *Pane) Sync(width, height int, selKey, contentKey string, contentFor func(width int) (content string, scrollTarget int, hasScrollTarget bool)) {
	p.VP.SetWidth(width)
	p.VP.SetHeight(height)

	selectionChanged := selKey != p.selKey
	p.selKey = selKey

	cacheHit := contentKey != "" && p.cache.valid && p.cache.key == contentKey && p.cache.width == width
	if !cacheHit {
		content, target, hasTarget := contentFor(width)
		p.cache = previewCache{key: contentKey, width: width, content: content, target: target, hasTarget: hasTarget, valid: contentKey != ""}
	}
	content, target, hasTarget := p.cache.content, p.cache.target, p.cache.hasTarget

	highlighted := p.Search.HasQuery()
	if highlighted {
		content = highlightPreviewContent(content, p.Search)
	}
	// Re-setting identical plain content is the per-Update cost the cache exists to avoid.
	if !cacheHit || highlighted || p.contentHighlighted {
		p.VP.SetContent(content)
	}
	p.contentHighlighted = highlighted

	if selectionChanged {
		if hasTarget {
			offset := target - p.VP.Height()/2
			p.VP.SetYOffset(max(offset, 0))
		} else {
			p.VP.GotoTop()
		}
		p.Search.SetMatches(nil)
	}
}

// Rect returns the preview panel's absolute on-screen bounds for a
// tab of the given width/contentHeight, computed from the same
// PanelWidths/PanelHeights/UseStackedLayout math each tab's View uses to lay
// out its panels - so click hit-testing stays in sync with what's actually
// rendered, in both tabs at once. A hidden preview has an empty rect.
func (p Pane) Rect(width, contentHeight int) (x, y, w, h int) {
	if p.Hidden {
		return 0, 0, 0, 0
	}
	sidebarW, previewW := SplitPanelWidth(width)
	sidebarH, previewH := SplitPanelHeight(width, contentHeight)
	if UseStackedLayout(width) {
		return 0, sidebarH + 1, previewW, previewH
	}
	return sidebarW + 1, 0, previewW, previewH
}

// InPreview bounds-checks a mouse click against the preview panel's rect (as
// returned by Rect for the given width/contentHeight), so the caller's
// row-under-click logic can skip a click that landed on the preview.
func (p *Pane) InPreview(mouse tea.Mouse, width, contentHeight int) bool {
	px, py, pw, ph := p.Rect(width, contentHeight)
	return mouse.X >= px && mouse.X < px+pw && mouse.Y >= py && mouse.Y < py+ph
}

// Result tells the parent model what a key meant beyond the pane's own
// state: the pane can't navigate away itself.
type Result int

const (
	Stay Result = iota
	Back        // "q": leave the tab
)

// Update processes a key while the preview has focus: its own search overlay,
// "q" asking the parent to go back, "h"/"left"/"esc" handing focus back to the
// sidebar, "b" jumping straight to the bottom (overriding the viewport's own
// "b" page-up — see bubbles/viewport's DefaultKeyMap), and everything else
// delegated to the viewport's own scrolling.
func (p *Pane) Update(msg tea.KeyPressMsg) (tea.Cmd, Result) {
	if handled, cmd := updatePreviewSearchKey(msg, &p.VP, &p.Search); handled {
		return cmd, Stay
	}
	switch msg.String() {
	case "q":
		return nil, Back
	case "h", "left", "esc":
		p.Focus = FocusSidebar
		return nil, Stay
	case "b":
		p.VP.GotoBottom()
		return nil, Stay
	}
	var cmd tea.Cmd
	p.VP, cmd = p.VP.Update(msg)
	return cmd, Stay
}
