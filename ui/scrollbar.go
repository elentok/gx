package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Scrollbar glyphs: a full block for the thumb over a thin track.
const (
	scrollbarThumbGlyph = "█"
	scrollbarTrackGlyph = "│"
)

var (
	styleScrollbarThumb = lipgloss.NewStyle().Foreground(ColorOverlay)
	styleScrollbarTrack = lipgloss.NewStyle().Foreground(ColorSurface1)
)

// RenderScrollbar returns a `height`-line vertical scrollbar column: a track with
// a proportional thumb whose size and position reflect how much of the content is
// visible and how far it is scrolled. It returns "" when the content fits
// (total <= visible) or when height <= 0, so callers can render it unconditionally.
//
// It is generic — no help-specific assumptions — so other overflowing views (the
// output modal, commit info header) can adopt it.
//
//	height  - number of lines the gutter occupies (the visible viewport height)
//	total   - total line count of the content
//	visible - number of lines visible at once
//	offset  - current scroll offset (top visible line index)
func RenderScrollbar(height, total, visible, offset int) string {
	if height <= 0 || total <= visible || visible <= 0 {
		return ""
	}

	// Thumb height proportional to the visible fraction, at least one line and at
	// most the full height.
	thumb := (height*visible + total - 1) / total // ceil(height*visible/total)
	thumb = min(max(thumb, 1), height)

	// Position the thumb proportional to the scroll offset. maxOffset is the
	// largest meaningful offset; clamp into [0, height-thumb].
	maxOffset := total - visible
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	travel := height - thumb
	pos := 0
	if maxOffset > 0 {
		pos = (travel*offset + maxOffset/2) / maxOffset // rounded
	}
	pos = min(max(pos, 0), travel)

	lines := make([]string, height)
	for i := range lines {
		if i >= pos && i < pos+thumb {
			lines[i] = styleScrollbarThumb.Render(scrollbarThumbGlyph)
		} else {
			lines[i] = styleScrollbarTrack.Render(scrollbarTrackGlyph)
		}
	}
	return strings.Join(lines, "\n")
}
