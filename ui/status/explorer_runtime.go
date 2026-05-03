package status

import "github.com/elentok/gx/ui/explorer"

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

func hunkDisplayBounds(sec sectionState, hunkIdx int) (start int, end int, ok bool) {
	return explorer.HunkDisplayBounds(sec.hunkDisplayRange, sec.parsed, sec.displayToRaw, hunkIdx)
}

func visualLineBounds(sec sectionState) (start, end int) {
	return explorer.VisualLineBounds(sec.visualAnchor, sec.activeLine, len(sec.parsed.Changed))
}
