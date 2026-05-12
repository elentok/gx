package diffview

import "charm.land/bubbles/v2/viewport"

func moveActive(section *DiffData, vp *viewport.Model, navMode NavMode, delta int, allowViewportScroll bool) bool {
	if navMode == NavModeHunk {
		if len(section.Parsed.Hunks) == 0 {
			return false
		}
		old := section.ActiveHunk
		if allowViewportScroll && section.ActiveHunk >= 0 && section.ActiveHunk < len(section.Parsed.Hunks) {
			if start, end, ok := section.HunkDisplayBounds(section.ActiveHunk); ok {
				visible := vp.VisibleLineCount()
				y := vp.YOffset()
				if visible > 0 {
					last := y + visible - 1
					if delta > 0 && end > last {
						vp.ScrollDown(1)
						return false
					}
					if delta < 0 && start < y {
						vp.ScrollUp(1)
						return false
					}
				}
			}
		}
		section.ActiveHunk += delta
		if section.ActiveHunk < 0 {
			section.ActiveHunk = 0
		}
		if section.ActiveHunk >= len(section.Parsed.Hunks) {
			section.ActiveHunk = len(section.Parsed.Hunks) - 1
		}
		return section.ActiveHunk != old
	}

	if len(section.Parsed.Changed) == 0 {
		return false
	}
	old := section.ActiveLine
	section.ActiveLine += delta
	if section.ActiveLine < 0 {
		section.ActiveLine = 0
	}
	if section.ActiveLine >= len(section.Parsed.Changed) {
		section.ActiveLine = len(section.Parsed.Changed) - 1
	}
	return section.ActiveLine != old
}

func scrollPage(vp *viewport.Model, direction int) {
	visible := vp.VisibleLineCount()
	if visible <= 0 {
		return
	}
	step := maxInt(1, visible/2)
	if direction > 0 {
		vp.ScrollDown(step)
	} else {
		vp.ScrollUp(step)
	}
}

func jumpTop(section *DiffData, vp *viewport.Model, navMode NavMode) bool {
	vp.SetYOffset(0)
	if navMode == NavModeHunk {
		if len(section.Parsed.Hunks) == 0 {
			return false
		}
		section.ActiveHunk = 0
		return true
	}
	if len(section.Parsed.Changed) == 0 {
		return false
	}
	section.ActiveLine = 0
	return true
}

func jumpBottom(section *DiffData, vp *viewport.Model, navMode NavMode) bool {
	maxOffset := vp.TotalLineCount() - vp.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	vp.SetYOffset(maxOffset)
	if navMode == NavModeHunk {
		if len(section.Parsed.Hunks) == 0 {
			return false
		}
		section.ActiveHunk = len(section.Parsed.Hunks) - 1
		return true
	}
	if len(section.Parsed.Changed) == 0 {
		return false
	}
	section.ActiveLine = len(section.Parsed.Changed) - 1
	return true
}
