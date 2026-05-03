package explorer

import "charm.land/bubbles/v2/viewport"

func ActiveRawLineIndex(section SectionData, navMode NavMode) int {
	if navMode == NavHunk {
		if section.ActiveHunk >= 0 && section.ActiveHunk < len(section.Parsed.Hunks) {
			return section.Parsed.Hunks[section.ActiveHunk].StartLine
		}
		return -1
	}
	if section.ActiveLine >= 0 && section.ActiveLine < len(section.Parsed.Changed) {
		return section.Parsed.Changed[section.ActiveLine].LineIndex
	}
	return -1
}

func EnsureActiveVisible(section SectionData, vp *viewport.Model, navMode NavMode) {
	if navMode == NavHunk && section.ActiveHunk >= 0 && section.ActiveHunk < len(section.HunkDisplayRange) {
		r := section.HunkDisplayRange[section.ActiveHunk]
		vp.EnsureVisible(r[0], 0, 0)
		return
	}
	if navMode == NavLine && section.ActiveLine >= 0 && section.ActiveLine < len(section.ChangedDisplay) && section.ChangedDisplay[section.ActiveLine] >= 0 {
		vp.EnsureVisible(section.ChangedDisplay[section.ActiveLine], 0, 0)
		return
	}
	active := ActiveRawLineIndex(section, navMode)
	if active >= 0 {
		display := active
		if active < len(section.RawToDisplay) && section.RawToDisplay[active] >= 0 {
			display = section.RawToDisplay[active]
		}
		vp.EnsureVisible(display, 0, 0)
	}
}

func MoveActive(section *SectionData, vp *viewport.Model, navMode NavMode, delta int, allowViewportScroll bool) bool {
	if navMode == NavHunk {
		if len(section.Parsed.Hunks) == 0 {
			return false
		}
		old := section.ActiveHunk
		if allowViewportScroll && section.ActiveHunk >= 0 && section.ActiveHunk < len(section.Parsed.Hunks) {
			if start, end, ok := HunkDisplayBounds(section.HunkDisplayRange, section.Parsed, section.DisplayToRaw, section.ActiveHunk); ok {
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

func ScrollPage(vp *viewport.Model, direction int) {
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

func JumpTop(section *SectionData, vp *viewport.Model, navMode NavMode) bool {
	vp.SetYOffset(0)
	if navMode == NavHunk {
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

func JumpBottom(section *SectionData, vp *viewport.Model, navMode NavMode) bool {
	maxOffset := vp.TotalLineCount() - vp.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	vp.SetYOffset(maxOffset)
	if navMode == NavHunk {
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
