package ui

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// ProgressBar draws cells glyphs: "▰" in fill for the done part, then "▱" in
// rest. It is full only at fraction 1; fraction is clamped to [0, 1].
func ProgressBar(fraction float64, cells int, fill, rest lipgloss.Style) string {
	if cells <= 0 {
		return ""
	}
	fraction = math.Min(math.Max(fraction, 0), 1)
	// The epsilon keeps exact ratios (7/7, 3/3) from losing a cell to float error.
	filled := min(int(math.Floor(fraction*float64(cells)+1e-9)), cells)
	if fraction < 1 {
		filled = min(filled, cells-1)
	}
	return fill.Render(strings.Repeat("▰", filled)) + rest.Render(strings.Repeat("▱", cells-filled))
}
