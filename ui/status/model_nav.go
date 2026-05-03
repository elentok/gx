package status

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

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

func (m *Model) scrollStatusPage(direction int) bool {
	if len(m.statusEntries) == 0 {
		return false
	}
	old := m.selected
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	statusH, _ := m.splitHeight(mainH)
	visible := maxInt(1, (statusH-2)/2)
	if direction > 0 {
		m.selected += visible
	} else {
		m.selected -= visible
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.statusEntries) {
		m.selected = len(m.statusEntries) - 1
	}
	if m.selected == old {
		return false
	}
	m.onStatusSelectionChanged()
	return true
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

func (m *Model) jumpToTop() {
	if m.focus == focusStatus {
		if len(m.statusEntries) == 0 {
			return
		}
		if m.selected == 0 {
			return
		}
		m.selected = 0
		m.onStatusSelectionChanged()
		return
	}
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

func (m *Model) jumpToBottom() {
	if m.focus == focusStatus {
		if len(m.statusEntries) == 0 {
			return
		}
		if m.selected == len(m.statusEntries)-1 {
			return
		}
		m.selected = len(m.statusEntries) - 1
		m.onStatusSelectionChanged()
		return
	}
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

func (m *Model) scheduleDiffReload() tea.Cmd {
	m.diffReloadSeq++
	seq := m.diffReloadSeq
	return tea.Tick(statusDiffReloadDebounce, func(time.Time) tea.Msg {
		return diffReloadMsg{seq: seq}
	})
}

func (m *Model) onStatusSelectionChanged() {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind == statusEntryDir {
		m.section = sectionUnstaged
		return
	}
	if entry.File.Path != m.activeFilePath {
		m.section = sectionUnstaged
	}
}
