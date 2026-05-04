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
	x := (screenW - fgW) / 2
	if x < 0 {
		x = 0
	}
	return PlaceOverlay(bg, fg, x, y)
}

// OverlayTopRight places fg flush against the top-right corner of bg.
func OverlayTopRight(bg, fg string, screenW int) string {
	fgW := lipgloss.Width(fg)
	x := screenW - fgW
	if x < 0 {
		x = 0
	}
	return PlaceOverlay(bg, fg, x, 0)
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
