package commit

import "github.com/elentok/gx/ui/explorer"

func (m *Model) diffPaneSize() (int, int) {
	headerH := maxInt(4, len(m.headerLines())+2)
	contentH := maxInt(5, m.height-1-headerH-1)
	if m.width < 90 {
		filesH := maxInt(5, contentH/3)
		diffH := maxInt(5, contentH-filesH)
		return m.width, diffH
	}
	leftW := maxInt(24, m.width/4)
	if leftW > m.width-40 {
		leftW = m.width - 40
	}
	if leftW < 24 {
		leftW = 24
	}
	return m.width - leftW, contentH
}

func (m *Model) syncDiffViewport() {
	diffW, diffH := m.diffPaneSize()
	bodyW := maxInt(1, diffW-4)
	bodyH := maxInt(0, diffH-2)
	explorer.ReflowSectionData(&m.section, bodyW, m.wrapSoft)
	m.diffViewport.SetWidth(bodyW)
	m.diffViewport.SetHeight(bodyH)
	m.diffViewport.SetContentLines(m.section.ViewLines)
	m.ensureActiveVisible()
}

func (m *Model) activeRawLineIndex() int {
	if m.diffNavMode == explorer.NavHunk {
		if m.section.ActiveHunk >= 0 && m.section.ActiveHunk < len(m.section.Parsed.Hunks) {
			return m.section.Parsed.Hunks[m.section.ActiveHunk].StartLine
		}
		return -1
	}
	if m.section.ActiveLine >= 0 && m.section.ActiveLine < len(m.section.Parsed.Changed) {
		return m.section.Parsed.Changed[m.section.ActiveLine].LineIndex
	}
	return -1
}

func (m *Model) ensureActiveVisible() {
	if m.diffNavMode == explorer.NavHunk && m.section.ActiveHunk >= 0 && m.section.ActiveHunk < len(m.section.HunkDisplayRange) {
		r := m.section.HunkDisplayRange[m.section.ActiveHunk]
		m.diffViewport.EnsureVisible(r[0], 0, 0)
		return
	}
	if m.diffNavMode == explorer.NavLine && m.section.ActiveLine >= 0 && m.section.ActiveLine < len(m.section.ChangedDisplay) && m.section.ChangedDisplay[m.section.ActiveLine] >= 0 {
		m.diffViewport.EnsureVisible(m.section.ChangedDisplay[m.section.ActiveLine], 0, 0)
		return
	}
	active := m.activeRawLineIndex()
	if active >= 0 {
		display := active
		if active < len(m.section.RawToDisplay) && m.section.RawToDisplay[active] >= 0 {
			display = m.section.RawToDisplay[active]
		}
		m.diffViewport.EnsureVisible(display, 0, 0)
	}
}

func (m *Model) moveDiffActive(delta int) {
	if m.diffNavMode == explorer.NavHunk {
		if len(m.section.Parsed.Hunks) == 0 {
			return
		}
		m.section.ActiveHunk += delta
		if m.section.ActiveHunk < 0 {
			m.section.ActiveHunk = 0
		}
		if m.section.ActiveHunk >= len(m.section.Parsed.Hunks) {
			m.section.ActiveHunk = len(m.section.Parsed.Hunks) - 1
		}
	} else {
		if len(m.section.Parsed.Changed) == 0 {
			return
		}
		m.section.ActiveLine += delta
		if m.section.ActiveLine < 0 {
			m.section.ActiveLine = 0
		}
		if m.section.ActiveLine >= len(m.section.Parsed.Changed) {
			m.section.ActiveLine = len(m.section.Parsed.Changed) - 1
		}
	}
	m.ensureActiveVisible()
}

func (m *Model) scrollDiffPage(direction int) {
	visible := m.diffViewport.VisibleLineCount()
	if visible <= 0 {
		return
	}
	step := maxInt(1, visible/2)
	if direction > 0 {
		m.diffViewport.ScrollDown(step)
	} else {
		m.diffViewport.ScrollUp(step)
	}
}

func (m *Model) jumpDiffTop() {
	m.diffViewport.SetYOffset(0)
	if m.diffNavMode == explorer.NavHunk {
		if len(m.section.Parsed.Hunks) == 0 {
			return
		}
		m.section.ActiveHunk = 0
		return
	}
	if len(m.section.Parsed.Changed) == 0 {
		return
	}
	m.section.ActiveLine = 0
}

func (m *Model) jumpDiffBottom() {
	maxOffset := m.diffViewport.TotalLineCount() - m.diffViewport.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.diffViewport.SetYOffset(maxOffset)
	if m.diffNavMode == explorer.NavHunk {
		if len(m.section.Parsed.Hunks) == 0 {
			return
		}
		m.section.ActiveHunk = len(m.section.Parsed.Hunks) - 1
		return
	}
	if len(m.section.Parsed.Changed) == 0 {
		return
	}
	m.section.ActiveLine = len(m.section.Parsed.Changed) - 1
}
