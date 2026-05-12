package diffview

import (
	"charm.land/bubbles/v2/viewport"
)

func restoreViewportYOffset(vp *viewport.Model, y int) {
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
