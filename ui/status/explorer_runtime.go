package status

func (m *Model) pickAvailableSection() {
	sections := m.visibleDiffSections()
	if len(sections) == 1 {
		m.section = sections[0]
	}
}

func (m Model) canSwitchSections() bool {
	return len(m.visibleDiffSections()) > 1
}

func (m *Model) currentSection() *sectionState {
	return m.sectionState(m.section)
}

func (m *Model) ensureActiveVisible(sec *sectionState) {
	if m.navMode == navHunk && sec.activeHunk >= 0 && sec.activeHunk < len(sec.hunkDisplayRange) {
		r := sec.hunkDisplayRange[sec.activeHunk]
		sec.viewport.EnsureVisible(r[0], 0, 0)
		return
	}
	if m.navMode == navLine && sec.activeLine >= 0 && sec.activeLine < len(sec.changedDisplay) && sec.changedDisplay[sec.activeLine] >= 0 {
		sec.viewport.EnsureVisible(sec.changedDisplay[sec.activeLine], 0, 0)
		return
	}
	active := m.activeRawLineIndex(*sec)
	if active >= 0 {
		display := active
		if active < len(sec.rawToDisplay) && sec.rawToDisplay[active] >= 0 {
			display = sec.rawToDisplay[active]
		}
		sec.viewport.EnsureVisible(display, 0, 0)
	}
}

func (m Model) editorLineForCurrentSelection() int {
	if m.focus != focusDiff {
		return 0
	}
	sec := m.currentSection()
	if m.navMode == navLine {
		if sec.activeLine < 0 || sec.activeLine >= len(sec.parsed.Changed) {
			return 0
		}
		cl := sec.parsed.Changed[sec.activeLine]
		if cl.NewLine > 0 {
			return cl.NewLine
		}
		return cl.OldLine
	}
	if sec.activeHunk < 0 || sec.activeHunk >= len(sec.parsed.Hunks) {
		return 0
	}
	h := sec.parsed.Hunks[sec.activeHunk]
	if h.NewStart > 0 {
		return h.NewStart
	}
	return h.OldStart
}

func restoreViewportYOffset(sec *sectionState, y int) {
	if y < 0 {
		y = 0
	}
	maxOffset := sec.viewport.TotalLineCount() - sec.viewport.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if y > maxOffset {
		y = maxOffset
	}
	sec.viewport.SetYOffset(y)
}

func hunkDisplayBounds(sec sectionState, hunkIdx int) (start int, end int, ok bool) {
	if hunkIdx >= 0 && hunkIdx < len(sec.hunkDisplayRange) {
		r := sec.hunkDisplayRange[hunkIdx]
		if r[0] >= 0 && r[1] >= r[0] {
			return r[0], r[1], true
		}
	}
	if hunkIdx < 0 || hunkIdx >= len(sec.parsed.Hunks) {
		return 0, 0, false
	}
	h := sec.parsed.Hunks[hunkIdx]
	start = -1
	end = -1
	for displayIdx, rawIdx := range sec.displayToRaw {
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

func visualLineBounds(sec sectionState) (start, end int) {
	start = sec.visualAnchor
	end = sec.activeLine
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if end >= len(sec.parsed.Changed) {
		end = len(sec.parsed.Changed) - 1
	}
	if start >= len(sec.parsed.Changed) {
		start = len(sec.parsed.Changed) - 1
	}
	if start < 0 {
		start = 0
	}
	return start, end
}
