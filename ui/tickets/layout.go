package tickets

import (
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tickets/preview"
)

// layout is the tab's panel geometry: the terminal size and the preview pane
// that splits it. It is built per call by Model.layout.
type layout struct {
	pane          *preview.Pane
	width, height int
}

func (m Model) layout() layout {
	return layout{pane: &m.preview, width: m.width, height: m.height}
}

func (l layout) contentHeight() int {
	return max(l.height, 4)
}

func (l layout) useStackedLayout() bool {
	return preview.UseStackedLayout(l.width)
}

func (l layout) splitWidth() (sidebarW, previewW int) {
	return l.pane.PanelWidths(l.width)
}

// splitHeight divides a stacked tickets view evenly between its selection-
// driving list and preview. Wide views remain a full-height side-by-side split.
func (l layout) splitHeight(total int) (sidebarH, previewH int) {
	return l.pane.PanelHeights(l.width, total)
}

// previewRect returns the preview panel's absolute on-screen bounds,
// mirroring normalView's layout math so mouse hit-testing (click-to-focus,
// wheel routing) stays in sync with what's actually rendered.
func (l layout) previewRect() (x, y, w, h int) {
	return l.pane.Rect(l.width, l.contentHeight())
}

// sidebarRect returns the sidebar panel's absolute on-screen bounds,
// mirroring previewRect's layout math so wheel hit-testing stays in sync
// with what's actually rendered.
func (l layout) sidebarRect() (x, y, w, h int) {
	sidebarW, _ := l.splitWidth()
	sidebarH, _ := l.splitHeight(l.contentHeight())
	return 0, 0, sidebarW, sidebarH
}

// sidebarViewportHeight is the sidebar body's visible line count, matching
// ui.RenderPanel's own bodyH math (PaddingY: 0, minus the header row) so the
// windowing done here lines up with what RenderPanel actually paints.
func (l layout) sidebarViewportHeight() int {
	sidebarH, _ := l.splitHeight(l.contentHeight())
	return max(sidebarH-1, 0)
}

func (l layout) searchOverlayWidth() int {
	return min(search.DESIRED_WIDTH, l.width*80/100)
}
