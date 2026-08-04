package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Scrollbar glyphs: both thumb and track are thin vertical lines - a heavy
// stroke for the thumb over a light stroke for the track - so the track
// stays visible along the full gutter height. That visible track is what
// lets the thumb's position read as "where in the whole list am I", not just
// "how much more is there".
const (
	scrollbarThumbGlyph = "┃"
	scrollbarTrackGlyph = "│"
)

var (
	styleScrollbarThumb = lipgloss.NewStyle().Foreground(ColorSubtle)
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

// ScrollbarGutter is the width a caller must reserve to the right of its
// content for AppendScrollbar's " " + bar column (mirrors the preview pane's
// own previewScrollbarGutter).
const ScrollbarGutter = 2

// AppendScrollbar pairs a windowed slice of already-visible content lines
// with a scroll indicator sized from the full scrollable region (total,
// visible, offset - see RenderScrollbar), truncating/padding each line to
// contentWidth-ScrollbarGutter so the appended " "+bar column lands at a
// fixed column instead of drifting with each line's own length. Returns
// lines unchanged when the content fits (RenderScrollbar returns "").
func AppendScrollbar(lines []string, contentWidth, total, visible, offset int) []string {
	bar := RenderScrollbar(len(lines), total, visible, offset)
	if bar == "" {
		return lines
	}
	barLines := strings.Split(bar, "\n")
	textWidth := max(contentWidth-ScrollbarGutter, 0)
	out := make([]string, len(lines))
	for i, line := range lines {
		text := ansi.Truncate(line, textWidth, "")
		text += strings.Repeat(" ", max(0, textWidth-ansi.StringWidth(text)))
		barSeg := ""
		if i < len(barLines) {
			barSeg = barLines[i]
		}
		out[i] = text + " " + barSeg
	}
	return out
}
