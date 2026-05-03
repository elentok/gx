package status

func (m *Model) moveActive(delta int) {
	sec := m.currentSection()
	if m.navMode == navHunk {
		if len(sec.parsed.Hunks) == 0 {
			return
		}
		old := sec.activeHunk
		if sec.activeHunk >= 0 && sec.activeHunk < len(sec.parsed.Hunks) {
			if start, end, ok := hunkDisplayBounds(*sec, sec.activeHunk); ok {
				visible := sec.viewport.VisibleLineCount()
				y := sec.viewport.YOffset()
				if visible > 0 {
					last := y + visible - 1
					if delta > 0 && end > last {
						sec.viewport.ScrollDown(1)
						return
					}
					if delta < 0 && start < y {
						sec.viewport.ScrollUp(1)
						return
					}
				}
			}
		}
		sec.activeHunk += delta
		if sec.activeHunk < 0 {
			sec.activeHunk = 0
		}
		if sec.activeHunk >= len(sec.parsed.Hunks) {
			sec.activeHunk = len(sec.parsed.Hunks) - 1
		}
		if sec.activeHunk == old {
			return
		}
	} else {
		if len(sec.parsed.Changed) == 0 {
			return
		}
		sec.activeLine += delta
		if sec.activeLine < 0 {
			sec.activeLine = 0
		}
		if sec.activeLine >= len(sec.parsed.Changed) {
			sec.activeLine = len(sec.parsed.Changed) - 1
		}
	}
	m.syncSearchCursorFromDiffFocus()
	m.ensureActiveVisible(sec)
}

func (m *Model) scrollDiffPage(direction int) {
	sec := m.currentSection()
	visible := sec.viewport.VisibleLineCount()
	if visible <= 0 {
		return
	}
	step := maxInt(1, visible/2)
	if direction > 0 {
		sec.viewport.ScrollDown(step)
	} else {
		sec.viewport.ScrollUp(step)
	}
}

func (m *Model) jumpDiffTop() {
	sec := m.currentSection()
	sec.viewport.SetYOffset(0)
	if m.navMode == navHunk {
		if len(sec.parsed.Hunks) == 0 {
			return
		}
		sec.activeHunk = 0
	} else {
		if len(sec.parsed.Changed) == 0 {
			return
		}
		sec.activeLine = 0
	}
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) jumpDiffBottom() {
	sec := m.currentSection()
	maxOffset := sec.viewport.TotalLineCount() - sec.viewport.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	sec.viewport.SetYOffset(maxOffset)
	if m.navMode == navHunk {
		if len(sec.parsed.Hunks) == 0 {
			return
		}
		sec.activeHunk = len(sec.parsed.Hunks) - 1
	} else {
		if len(sec.parsed.Changed) == 0 {
			return
		}
		sec.activeLine = len(sec.parsed.Changed) - 1
	}
	m.syncSearchCursorFromDiffFocus()
}
