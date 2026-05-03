package explorer

import (
	"charm.land/bubbles/v2/viewport"

	"github.com/elentok/gx/ui/diff"
)

func RestoreViewportYOffset(vp *viewport.Model, y int) {
	if y < 0 {
		y = 0
	}
	maxOffset := vp.TotalLineCount() - vp.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if y > maxOffset {
		y = maxOffset
	}
	vp.SetYOffset(y)
}

func HunkDisplayBounds(
	hunkDisplayRange [][2]int,
	parsed diff.ParsedDiff,
	displayToRaw []int,
	hunkIdx int,
) (start int, end int, ok bool) {
	if hunkIdx >= 0 && hunkIdx < len(hunkDisplayRange) {
		r := hunkDisplayRange[hunkIdx]
		if r[0] >= 0 && r[1] >= r[0] {
			return r[0], r[1], true
		}
	}
	if hunkIdx < 0 || hunkIdx >= len(parsed.Hunks) {
		return 0, 0, false
	}
	h := parsed.Hunks[hunkIdx]
	start = -1
	end = -1
	for displayIdx, rawIdx := range displayToRaw {
		if rawIdx < h.StartLine || rawIdx > h.EndLine {
			continue
		}
		if start < 0 {
			start = displayIdx
		}
		end = displayIdx
	}
	if start < 0 || end < 0 {
		return 0, 0, false
	}
	return start, end, true
}

func VisualLineBounds(visualAnchor, activeLine, changedCount int) (start, end int) {
	start = visualAnchor
	end = activeLine
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if end >= changedCount {
		end = changedCount - 1
	}
	if start >= changedCount {
		start = changedCount - 1
	}
	if start < 0 {
		start = 0
	}
	return start, end
}
