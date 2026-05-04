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

func (m *Model) cycleFrameForward() {
	if m.focus == focusStatus {
		m.focus = focusDiff
		m.pickAvailableSection()
		m.syncDiffViewports()
		m.ensureActiveVisible(m.currentSection())
		return
	}
	if m.section == sectionUnstaged && m.canSwitchSections() {
		m.section = sectionStaged
		m.syncDiffViewports()
		m.ensureActiveVisible(m.currentSection())
		return
	}
	m.focus = focusStatus
}

func (m *Model) currentSection() *sectionState {
	return m.sectionState(m.section)
}

func (m *Model) ensureActiveVisible(sec *sectionState) {
	explorer.EnsureActiveVisible(toExplorerSectionData(*sec), &sec.viewport, m.navMode)
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
