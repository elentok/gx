package queue

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tree"
)

// queueLayout is the Queue tab's panel geometry: the terminal size and the
// preview pane that splits it. It is built per call by QueueModel.layout.
type queueLayout struct {
	pane          *preview.Pane
	width, height int
}

// layout builds the tab's current geometry.
func (m *QueueModel) layout() queueLayout {
	return queueLayout{pane: &m.preview, width: m.width, height: m.height}
}

// contentHeight mirrors Model.contentHeight for the Queue tab's own layout.
func (l queueLayout) contentHeight() int { return max(l.height, 1) }

// treeRect is the queue tree panel's absolute on-screen bounds.
func (l queueLayout) treeRect() ui.Rect {
	sidebarW, _ := l.pane.PanelWidths(l.width)
	sidebarH, _ := l.pane.PanelHeights(l.width, l.contentHeight())
	return ui.Rect{X: 0, Y: 0, W: sidebarW, H: sidebarH}
}

// previewRect is the preview panel's absolute on-screen bounds.
func (l queueLayout) previewRect() ui.Rect {
	x, y, w, h := l.pane.Rect(l.width, l.contentHeight())
	return ui.Rect{X: x, Y: y, W: w, H: h}
}

// viewportHeight is the queue panel's visible body line count, matching
// ui.RenderPanel's own bodyH math (PaddingY: 0, minus the header row), so the
// windowing done here lines up with what RenderPanel paints. A stacked
// (narrow-terminal) layout shares the height with the preview pane below it.
func (l queueLayout) viewportHeight() int {
	sidebarH, _ := l.pane.PanelHeights(l.width, l.contentHeight())
	return max(sidebarH-1, 0)
}

// bodyWidth is the width of the Queue panel's body: the sidebar less its two
// border columns.
func (l queueLayout) bodyWidth() int {
	sidebarW, _ := l.pane.PanelWidths(l.width)
	return sidebarW - 2
}

// labelWidth is the width a Queue row's label can use inside the body. Zero or
// negative widths (before the first resize) mean "unconstrained".
func (l queueLayout) labelWidth() int {
	if l.width <= 0 {
		return 0
	}
	return tree.LabelWidth(l.bodyWidth())
}
