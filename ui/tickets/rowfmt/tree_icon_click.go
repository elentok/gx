package rowfmt

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tree"

	"charm.land/lipgloss/v2"
)

// PanelPaddingX is the inner gap ui.PanelOptionsFor puts left of the body.
const PanelPaddingX = 1

// OnTreeIcon reports whether screen column mouseX hits the entry's
// expand/collapse glyph: the glyph, the space after it, and one cell of slop
// on the left. labelOffset is the cells the row's label puts before the glyph.
// The panels sit at screen column 0.
func OnTreeIcon[T any](mouseX int, entry tree.Entry[T], labelOffset int, icons ui.IconSet) bool {
	glyphW := max(lipgloss.Width(icons.TriangleExpanded), lipgloss.Width(icons.TriangleCollapsed))
	start := PanelPaddingX + tree.LabelColumn(entry) + labelOffset
	return mouseX >= start-1 && mouseX <= start+glyphW
}
