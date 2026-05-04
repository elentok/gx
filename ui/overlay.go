package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func OverlayCenter(bg, fg string, screenW, screenH int) string {
	fgW := lipgloss.Width(fg)
	fgH := lipgloss.Height(fg)
	x := (screenW - fgW) / 2
	y := (screenH - fgH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return PlaceOverlay(bg, fg, x, y)
}

// OverlayBottomCenter places fg horizontally centered at the given y coordinate.
func OverlayBottomCenter(bg, fg string, screenW, y int) string {
	fgW := lipgloss.Width(fg)
	x := max((screenW-fgW)/2, 0)
	return PlaceOverlay(bg, fg, x, y)
}

// OverlayBottomRight places fg against the top-right corner of bg (1px margin).
func OverlayTopRight(bg, fg string, screenW int) string {
	fgW := lipgloss.Width(fg)
	x := max(screenW-fgW-1, 0)
	return PlaceOverlay(bg, fg, x, 1)
}

// OverlayTopRight places fg against the bottom-right corner of bg
// (1px horizontal margin and 2px vertical margin)
func OverlayBottomRight(bg, fg string, screenW int, screenH int) string {
	fgW := lipgloss.Width(fg)
	fgH := lipgloss.Height(fg)
	x := max(screenW-fgW-1, 0)
	y := max(screenH-fgH-2, 0)

	return PlaceOverlay(bg, fg, x, y)
}

func PlaceOverlay(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fgLine := range fgLines {
		bgY := y + i
		if bgY < 0 || bgY >= len(bgLines) {
			continue
		}
		bgLine := bgLines[bgY]
		fgW := ansi.StringWidth(fgLine)

		left := ansi.Truncate(bgLine, x, "")
		if leftW := ansi.StringWidth(left); leftW < x {
			left += strings.Repeat(" ", x-leftW)
		}
		right := ansi.TruncateLeft(bgLine, x+fgW, "")
		bgLines[bgY] = left + fgLine + right
	}

	return strings.Join(bgLines, "\n")
}
