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
		switch {
		case m.sectionHasContent(sectionUnstaged):
			m.section = sectionUnstaged
		case m.sectionHasContent(sectionStaged):
			m.section = sectionStaged
		default:
			m.pickAvailableSection()
		}
		m.syncDiffViewports()
		m.ensureActiveVisible(m.currentSection())
		return
	}
	if m.canSwitchSections() {
		if m.section == sectionUnstaged && m.sectionHasContent(sectionStaged) {
			m.section = sectionStaged
			m.syncDiffViewports()
			m.ensureActiveVisible(m.currentSection())
			return
		}
	}
	m.focus = focusStatus
}

func (m *Model) currentSection() *sectionState {
	return m.sectionState(m.section)
}

func (m *Model) ensureActiveVisible(sec *sectionState) {
	explorer.EnsureActiveVisible(sec.data, &sec.viewport, m.navMode)
}

func (m Model) editorLineForCurrentSelection() int {
	if m.focus != focusDiff {
		return 0
	}
	sec := m.currentSection()
	if m.navMode == navLine {
		if sec.data.ActiveLine < 0 || sec.data.ActiveLine >= len(sec.data.Parsed.Changed) {
			return 0
		}
		cl := sec.data.Parsed.Changed[sec.data.ActiveLine]
		if cl.NewLine > 0 {
			return cl.NewLine
		}
		return cl.OldLine
	}
	if sec.data.ActiveHunk < 0 || sec.data.ActiveHunk >= len(sec.data.Parsed.Hunks) {
		return 0
	}
	h := sec.data.Parsed.Hunks[sec.data.ActiveHunk]
	if h.NewStart > 0 {
		return h.NewStart
	}
	return h.OldStart
}

func hunkDisplayBounds(sec sectionState, hunkIdx int) (start int, end int, ok bool) {
	return explorer.HunkDisplayBounds(sec.data.HunkDisplayRange, sec.data.Parsed, sec.data.DisplayToRaw, hunkIdx)
}

func visualLineBounds(sec sectionState) (start, end int) {
	return explorer.VisualLineBounds(sec.data.VisualAnchor, sec.data.ActiveLine, len(sec.data.Parsed.Changed))
}
