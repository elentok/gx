package help

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui"
)

const (
	// targetColWidth is the per-column width the packer aims for when deciding how
	// many columns fit in the modal. Real columns run ~16–30 wide, so this is a
	// deliberate underestimate of the modal width each column needs.
	targetColWidth = 28
	// maxColumns caps the responsive column count.
	maxColumns = 4
	// colGap is the horizontal space between adjacent columns.
	colGap = 2
)

// columnCount returns the responsive number of columns for the given modal width:
// clamp(width / targetColWidth, 1, maxColumns).
func columnCount(width int) int {
	return min(max(width/targetColWidth, 1), maxColumns)
}

// sectionHeight is the line count a section block occupies: one heading line plus
// one line per binding.
func sectionHeight(s KeySection) int {
	return 1 + len(s.Bindings)
}

// packColumns distributes whole section-blocks column-major into `cols` columns,
// keeping each section intact (never split, never orphan a heading). It breaks to
// the next column once the accumulated height crosses ceil(totalHeight / cols),
// leaving the remainder in the final column. It is a pure function so the layout
// can be tested without rendering.
func packColumns(sections []KeySection, cols int) [][]KeySection {
	cols = max(cols, 1)
	if len(sections) == 0 {
		return nil
	}

	total := 0
	for _, s := range sections {
		total += sectionHeight(s)
	}
	threshold := ceilDiv(total, cols)

	columns := make([][]KeySection, 0, cols)
	var cur []KeySection
	curHeight := 0
	for _, s := range sections {
		h := sectionHeight(s)
		// Break to the next column before placing a section that would push the
		// current column past the threshold, so all `cols` columns get filled
		// instead of a few tall columns swallowing the budget. The last column
		// (len(columns) == cols-1) stays open for the remainder.
		if curHeight > 0 && curHeight+h > threshold && len(columns) < cols-1 {
			columns = append(columns, cur)
			cur = nil
			curHeight = 0
		}
		cur = append(cur, s)
		curHeight += h
	}
	if len(cur) > 0 {
		columns = append(columns, cur)
	}
	return columns
}

func ceilDiv(a, b int) int {
	if b <= 0 {
		return a
	}
	return (a + b - 1) / b
}

// RenderColumns lays the sections out in a responsive multi-column block sized to
// `width`. Each section stays intact; sections flow column-major; columns are
// padded to equal width and joined horizontally.
func RenderColumns(sections []KeySection, width int) string {
	cols := columnCount(width)
	columns := packColumns(sections, cols)
	if len(columns) == 0 {
		return ""
	}

	// Render each column at its own natural width (lipgloss.JoinHorizontal pads
	// each block to its widest line) so narrow columns stay tight instead of being
	// bloated to the widest column.
	gap := strings.Repeat(" ", colGap)
	withGaps := make([]string, 0, len(columns)*2-1)
	for i, colSections := range columns {
		if i > 0 {
			withGaps = append(withGaps, gap)
		}
		withGaps = append(withGaps, renderColumn(colSections))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, withGaps...)
}

// renderColumn renders one column's sections (heading + aligned key/title rows)
// joined vertically, with a blank line between sections.
func renderColumn(sections []KeySection) string {
	keyStyle := ui.StyleTitle
	descStyle := ui.StyleHint
	sep := descStyle.Render("  ")

	var lines []string
	for si, section := range sections {
		if si > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, ui.StyleHelpHeading.Render(section.Title))

		keyW := 0
		for _, b := range section.Bindings {
			if w := ansi.StringWidth(b.Keys()); w > keyW {
				keyW = w
			}
		}
		for _, b := range section.Bindings {
			pad := strings.Repeat(" ", keyW-ansi.StringWidth(b.Keys()))
			lines = append(lines, "  "+keyStyle.Render(b.Keys())+pad+sep+descStyle.Render(b.Title))
		}
	}
	return strings.Join(lines, "\n")
}
